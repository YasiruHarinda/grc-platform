// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"net/http"
	"strconv"

	"context"
	"fmt"
	"log/slog"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleEscalateRisk serves POST /api/v1/risks/{id}/escalate — the manual
// trigger for Compliance/Admin to escalate an overdue IN_REMEDIATION risk on
// demand, rather than waiting for the daily job (up to 24h delay) to reach
// it. Same outcome as the automatic path: OPEN escalation created, risk
// flips to ESCALATED. The entity re-validates IN_REMEDIATION + overdue, so a
// risk that's already moved on (e.g. someone just closed it, or the job beat
// this click to it) returns a clear 4xx rather than being escalated wrongly.
func (d *Deps) handleEscalateRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	registerID, err := d.sourceRegisterOf(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilegeIn(r.Context(), w, privilege.EscalateRisk, registerID) {
		return
	}
	escalation, err := d.Escalation.Escalate(r.Context(), riskID, by)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), riskID, by, model.HistoryEscalate, model.HistoryDetails{
		From: model.StatusInRemediation, To: model.StatusEscalated,
	})
	d.NotifyEscalation(r.Context(), riskID, by)
	response.WriteJSONValue(w, http.StatusOK, escalation)
}

// handleListEscalations serves GET /api/v1/risks/{id}/escalations. Visible to
// anyone who can view the risk — escalation history is system-generated (see
// model.Escalation) and shown the same as any other risk field — except an
// Action-Owner-only caller, who is further scoped to risks where they own a
// plan (riskVisibleToCaller), matching handleListRisks' list scoping.
func (d *Deps) handleListEscalations(w http.ResponseWriter, r *http.Request) {
	// Unscoped on purpose: this gates only whether the caller may read risks at
	// all. WHICH risks they may read is decided by riskVisibleToCaller / the
	// list scoping, not by this privilege.
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	visible, err := d.riskVisibleToCaller(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !visible {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	escalations, err := d.Escalation.List(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, escalations)
}

// handleEscalationComment serves
// POST /api/v1/risks/{id}/escalations/{escalationId}/comment.
//
// This replaces the MANAGEMENT action plan as the way an escalation is
// answered: a comment alone returns the risk to its assigner. The assigner may
// then add further action plans, but nothing forces them to.
//
// Deliberately not gated on any privilege — requireCallerUUID below still
// requires a valid authenticated session, but nothing more. For a medium/low
// risk the entitled commenter is a line manager, who holds no risk role by
// virtue of managing someone, and per authorizeComment's own comment need not
// even be a platform user — a privilege gate would reject exactly the people
// this endpoint exists for before the real check ever ran. That real check is
// the identity one in the service, which matches the caller against the
// risk's Management Approver (HIGH) or the leads frozen on the escalation row
// (MEDIUM/LOW), by email.
func (d *Deps) handleEscalationComment(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	escalationID, err := strconv.Atoi(r.PathValue("escalationId"))
	if err != nil || escalationID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "escalationId must be a positive integer")
		return
	}

	var req model.EscalationCommentRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	registerID, err := d.sourceRegisterOf(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	escalation, err := d.Escalation.Comment(r.Context(), riskID, escalationID, req.Comment, by,
		canOverrideAssigneeIn(r.Context(), registerID))
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	d.recordEvent(r.Context(), riskID, by, model.HistoryComment, model.HistoryDetails{
		From: model.StatusEscalated, To: model.StatusInRemediation, Comment: req.Comment,
	})
	// The risk is back with its assigner, who has to act on the comment.
	detail, err := d.Risk.GetByID(r.Context(), riskID)
	if err == nil {
		d.notifyRiskEvent(emailer.EventEscalationCommented, riskID, []int{detail.AssignerID}, by, req.Comment)
	}
	response.WriteJSONValue(w, http.StatusOK, escalation)
}

// NotifyEscalation emails the people who need to know a risk has blown its
// deadline. Who that is depends on the risk's level:
//
//   - HIGH:          the Management Approver, plus the assigner, action
//     owner(s) and risk owner.
//   - MEDIUM / LOW:  the assigner, action owner(s) and risk owner. Management
//     is not troubled by a lower-level slip.
//
// One intended recipient is deliberately NOT mailed yet — the assigner's and
// action owner's line managers. The leads were explicitly deferred, and it is
// logged so the trigger point stays observable. Their Asgardeo ids are
// already frozen on the escalation row, so enabling them later is a
// recipient list change here (resolve uuid -> email via the identity
// directory), not a data change.
//
// TODO: notify the two leads recorded on the escalation row
// (assigner_lead_uuid / action_owner_lead_uuid), once it is decided that
// leads should be emailed.
//
// Exported because the daily escalation job calls it too (via
// NotifyEscalationSync below) — automatic and manual escalations must notify
// identically, and the surest way to guarantee that is for them to share the
// same recipient resolution.
//
// Fire-and-forget: safe here because this runs on the request path, and a
// caller that never learns the outcome is fine — a human clicked Escalate and
// can see it worked. The daily job cannot make that assumption; see
// NotifyEscalationSync.
func (d *Deps) NotifyEscalation(ctx context.Context, riskID int, by string) {
	recipients, registerID, err := d.escalationRecipients(ctx, riskID)
	if err != nil {
		slog.Warn("escalation notification: failed to load risk", "riskId", riskID, "err", err)
		return
	}
	d.notifyRiskEvent(emailer.EventEscalated, riskID, recipients, by, "")
	d.notifyComplianceAdmins(emailer.EventEscalated, riskID, registerID, by, "")
	notifyEscalationLeads(riskID)
}

// NotifyEscalationSync is NotifyEscalation's synchronous counterpart, used
// only by the daily escalation job (internal/risk/job). The job can afford to
// wait for the actual send to finish — it has no response to protect the way
// an HTTP handler does — and it needs to: a fire-and-forget notification whose
// caller never learns whether it succeeded is exactly what let a run of the
// job log "escalated 40" while silently sending zero emails, with no retry,
// since Escalate's status flip already made every one of those risks
// ineligible for the job's query on the next run.
func (d *Deps) NotifyEscalationSync(ctx context.Context, riskID int, by string) error {
	recipients, registerID, err := d.escalationRecipients(ctx, riskID)
	if err != nil {
		return fmt.Errorf("load risk for notification: %w", err)
	}
	sendErr := d.sendRiskEventSync(ctx, emailer.EventEscalated, riskID, recipients, by, "")
	d.notifyComplianceAdmins(emailer.EventEscalated, riskID, registerID, by, "")
	notifyEscalationLeads(riskID)
	return sendErr
}

// escalationRecipients resolves who a risk's escalation should notify: the
// assigner, owner, and action owner(s) always, plus the Management Approver
// when the risk is HIGH. Shared by NotifyEscalation and NotifyEscalationSync
// so the two can never resolve a different recipient list for the same risk.
//
// Also returns the risk's source register, which the caller needs to notify
// compliance admins for the same risk — loading detail twice for one
// notification would be wasted work.
func (d *Deps) escalationRecipients(ctx context.Context, riskID int) (recipients []int, registerID int, err error) {
	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		return nil, 0, err
	}

	recipients = []int{detail.AssignerID, detail.OwnerID}
	if plans, err := d.ActionPlan.List(ctx, riskID); err == nil {
		for _, p := range plans {
			if p.ActionOwnerID != nil {
				recipients = append(recipients, *p.ActionOwnerID)
			}
		}
	} else {
		slog.Warn("escalation notification: failed to list plans", "riskId", riskID, "err", err)
	}

	// Effective level, not gross — the same level the registers table shows,
	// and the same one authorizeComment uses to decide who may respond.
	level := ""
	if detail.EffectiveScore != nil {
		level = detail.EffectiveScore.RiskLevel
	} else if detail.GrossScore != nil {
		level = detail.GrossScore.RiskLevel
	}
	if level == "HIGH" {
		recipients = append(recipients, detail.ManagementApproverID)
	}

	return recipients, detail.SourceRegisterID, nil
}

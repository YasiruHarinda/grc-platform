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
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.EscalateRisk) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
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
// Deliberately gated on RISK_VIEW_RISKS rather than a dedicated privilege. For
// a medium/low risk the entitled commenter is a line manager, who holds no risk
// role by virtue of managing someone — a privilege gate would lock out exactly
// the people this endpoint exists for. The real check is the identity one in
// the service, which matches the caller against the risk's Management Approver
// (HIGH) or the leads frozen on the escalation row (MEDIUM/LOW).
func (d *Deps) handleEscalationComment(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
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

	escalation, err := d.Escalation.Comment(r.Context(), riskID, escalationID, req.Comment, by, canOverrideAssignee(r.Context()))
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
// Three intended recipients are deliberately NOT mailed yet — the compliance
// admin role, and the assigner's and action owner's line managers. The role
// would fan out to everyone holding it, and the leads were explicitly deferred;
// both are logged so the trigger point stays observable. The lead emails are
// already frozen on the escalation row, so enabling them later is a recipient
// list change here, not a data change.
//
// TODO: notify the compliance admin role, and the two leads recorded on the
// escalation row (assigner_lead_email / action_owner_lead_email), once it is
// decided who should be on those lists.
//
// Exported because the daily escalation job calls it too — automatic and manual
// escalations must notify identically, and the surest way to guarantee that is
// for them to share this one function.
func (d *Deps) NotifyEscalation(ctx context.Context, riskID int, by string) {
	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("escalation notification: failed to load risk", "riskId", riskID, "err", err)
		return
	}

	recipients := []int{detail.AssignerID, detail.OwnerID}
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

	d.notifyRiskEvent(emailer.EventEscalated, riskID, recipients, by, "")
	notifyComplianceAdmins(emailer.EventEscalated, riskID)
	notifyEscalationLeads(riskID)
}

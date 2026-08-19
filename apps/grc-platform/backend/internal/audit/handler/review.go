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
	"context"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// requireAssignedAuditor authorizes population-validation, sample-selection, and
// evidence-validation actions to the control's assigned auditor POC only —
// matched by email against control.AuditorEmail — since those are external-
// auditor decisions, not internal-reviewer ones.
//
// requiredPriv is the auditor privilege for the action — AUDIT_VALIDATE_EVIDENCE
// for the validate endpoints, AUDIT_SELECT_SAMPLE for the sample endpoints. It
// layers on top of the assigned-auditor scope: the caller must both
// hold the privilege AND be the control's assigned auditor. Requiring the
// specific privilege (stronger than the old baseline ViewAudits check) also
// stops a token with zero auditor grants from acting by an auditor_id coincidence.
//
// ManageControls holders bypass the scope check, consistent with requireAssignment
// elsewhere: they already have full read/write over all audit data.
func requireAssignedAuditor(w http.ResponseWriter, r *http.Request, control *model.AuditControl, requiredPriv string) bool {
	if auth.HasPrivilege(r.Context(), privilege.ManageControls) {
		return true
	}
	if !auth.RequirePrivilege(r.Context(), w, requiredPriv) {
		return false
	}
	actor := auth.FromContext(r.Context())
	if control.AuditorEmail == nil || !strings.EqualFold(*control.AuditorEmail, actor.Email) {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return false
	}
	return true
}

// assignedAuditorGate adapts requireAssignedAuditor to the decideRound postGate
// signature, binding the auditor privilege the action requires.
func assignedAuditorGate(requiredPriv string) func(http.ResponseWriter, *http.Request, *model.AuditControl) bool {
	return func(w http.ResponseWriter, r *http.Request, control *model.AuditControl) bool {
		return requireAssignedAuditor(w, r, control, requiredPriv)
	}
}

// decideRoundParams configures decideRound for one of the four review/validate
// endpoints (evidence or population, at either the internal-review or the
// auditor-validation stage). The four handlers differ only in these fields —
// everything else (fetch control, guard status, decode the decision, fetch
// the round, commit round status then control status in that order) lives
// once in decideRound, so that order can't silently diverge between evidence
// and population again. A rejected round's files are never auto-cleared —
// they stay on record (visible/deletable, same as evidence) so the team can
// see what was rejected and clean it up themselves before resubmitting.
type decideRoundParams struct {
	// preGate runs before the control is fetched — for checks that don't need
	// it (the internal reviewer's plain privilege check). nil to skip.
	preGate func(w http.ResponseWriter, r *http.Request) bool
	// postGate runs after the control is fetched — for checks that need it
	// (requireAssignedAuditor, keyed on control.AuditorEmail). nil to skip.
	postGate func(w http.ResponseWriter, r *http.Request, control *model.AuditControl) bool

	requiredStatus    string // the control.Status this action is only valid from
	statusConflictMsg string

	latestRoundID     func(ctx context.Context, auditID, controlID int) (int, error)
	updateRoundStatus func(ctx context.Context, roundID int, status, updatedBy string) error

	approveRoundStatus, approveControlStatus string
	rejectRoundStatus, rejectControlStatus   string
}

// decideRound implements the shared shape of an internal-review or
// auditor-validation decision: gate → fetch control → guard status → decode
// APPROVE/REJECT → fetch the latest round → commit round status → commit
// control status → (reject only) best-effort file cleanup → respond.
func (h *evidenceHandler) decideRound(w http.ResponseWriter, r *http.Request, p decideRoundParams) {
	if p.preGate != nil && !p.preGate(w, r) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control == nil {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	if p.postGate != nil && !p.postGate(w, r, control) {
		return
	}
	if control.Status != p.requiredStatus {
		response.WriteError(w, http.StatusConflict, p.statusConflictMsg)
		return
	}
	req, ok := decodeReviewDecision(w, r)
	if !ok {
		return
	}

	roundID, err := p.latestRoundID(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	actor := auth.FromContext(r.Context()).Email
	roundStatus, controlStatus := p.approveRoundStatus, p.approveControlStatus
	reject := req.Decision == "REJECT"
	if reject {
		roundStatus, controlStatus = p.rejectRoundStatus, p.rejectControlStatus
	}
	if err := p.updateRoundStatus(r.Context(), roundID, roundStatus, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	statusReq := model.UpdateStatusRequest{Status: controlStatus, Comment: req.Comment}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"status": controlStatus})
}

// decodeReviewDecision reads {"decision":"APPROVE"|"REJECT","comment":"..."} and
// normalizes decision to upper case. Writes a 400 and returns ok=false on a
// missing body or an unrecognized decision value.
func decodeReviewDecision(w http.ResponseWriter, r *http.Request) (model.ReviewDecisionRequest, bool) {
	var req model.ReviewDecisionRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return req, false
	}
	req.Decision = strings.ToUpper(req.Decision)
	if req.Decision != "APPROVE" && req.Decision != "REJECT" {
		response.WriteError(w, http.StatusBadRequest, `decision must be "APPROVE" or "REJECT"`)
		return req, false
	}
	return req, true
}

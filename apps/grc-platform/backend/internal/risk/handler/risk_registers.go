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
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// requireUserEmail extracts the caller's email and writes a 401 when the
// request carries no authenticated user. Returns ("", false) on failure.
func requireUserEmail(w http.ResponseWriter, r *http.Request) (string, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return "", false
	}
	if user.Email != "" {
		return user.Email, true
	}
	return user.Subject, true
}

// parseRiskID extracts and validates the {id} path parameter.
func parseRiskID(w http.ResponseWriter, r *http.Request) (int, bool) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "invalid risk id")
		return 0, false
	}
	return id, true
}

// splitCSV splits a comma-separated query param into trimmed, non-empty parts.
func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// splitCSVInts is splitCSV for comma-separated integer IDs; non-numeric or
// non-positive entries are silently dropped rather than erroring the request.
func splitCSVInts(raw string) []int {
	var out []int
	for _, s := range splitCSV(raw) {
		if id, err := strconv.Atoi(s); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// isActionOwnerOnly reports whether the caller holds CompleteActionSteps but
// none of the broader viewer privileges — the privilege-driven equivalent of
// "this is an Action Owner, not a Risk Assigner/Owner/Compliance/Management/
// Admin user," without checking a role name (this module never does).
//
// broader is a hand-maintained allowlist, not derived from the full privilege
// set — a future write/approve privilege that isn't added here would silently
// mis-classify a broad holder as action-owner-only, over-scoping them (see
// riskVisibleToCaller and its handleListRisks / by-id read callers). When you
// add a new Risk-module write or approval privilege, add it to this list too.
func isActionOwnerOnly(ctx context.Context) bool {
	if !auth.HasPrivilege(ctx, privilege.CompleteActionSteps) {
		return false
	}
	// Keep in sync with every write/approval privilege in
	// internal/shared/privilege/privilege.go's Risk Hub block.
	broader := []string{
		privilege.CreateRisk,
		privilege.UpdateRisk,
		privilege.SubmitRisk,
		privilege.CancelRisk,
		privilege.OwnerApproveRisk,
		privilege.ManagementApproveRisk,
		privilege.ComplianceApproveRisk,
		privilege.OwnerRejectRisk,
		privilege.ManagementRejectRisk,
		privilege.ComplianceRejectRisk,
		privilege.CompleteRisk,
		privilege.CloseRisk,
		privilege.EscalateRisk,
		privilege.AssessRisk,
		privilege.ManageTeams,
		privilege.ManageRiskScores,
		privilege.ManageActionPlans,
		privilege.ManageComplianceRefs,
		privilege.CreateManagementActionPlan,
	}
	for _, p := range broader {
		if auth.HasPrivilege(ctx, p) {
			return false
		}
	}
	return true
}

// seesEveryRisk is the hand-maintained allowlist of privileges that are only
// ever granted to Compliance/Management/Admin (never to a plain Risk Assigner
// or Risk Owner) — see shared_seed_data.sql's role_privilege grants. Holding
// any one of these means the caller sees every risk, unscoped.
//
// Like isActionOwnerOnly's broader list above, this must be kept in sync by
// hand: a new Compliance/Management/Admin-only privilege that isn't added here
// would wrongly leave its holder team-scoped instead of seeing everything.
var seesEveryRisk = []string{
	privilege.ViewAllRisks,
	privilege.ComplianceApproveRisk,
	privilege.ComplianceRejectRisk,
	privilege.CloseRisk,
	privilege.EscalateRisk,
	privilege.ManageComplianceRefs,
	privilege.ManagementApproveRisk,
	privilege.ManagementRejectRisk,
	privilege.CreateManagementActionPlan,
	privilege.ManageTeams,
	privilege.ManageRiskScores,
}

// isTeamScopedOnly reports whether the caller should be scoped to risks
// belonging to their own risk teams — true for a Risk Assigner or Risk Owner
// who holds none of the Compliance/Management/Admin-only privileges in
// seesEveryRisk. Explicitly excludes Action-Owner-only callers (handled by
// isActionOwnerOnly's own, narrower scoping instead) so the two never overlap:
// without this check, someone holding only VIEW_RISKS + COMPLETE_ACTION_STEPS
// would satisfy both, since neither privilege appears in seesEveryRisk.
// Classifying by privilege (not role name) matches isActionOwnerOnly and the
// rest of this module's convention.
func isTeamScopedOnly(ctx context.Context) bool {
	if isActionOwnerOnly(ctx) {
		return false
	}
	for _, p := range seesEveryRisk {
		if auth.HasPrivilege(ctx, p) {
			return false
		}
	}
	return true
}

// callerUserID resolves the authenticated caller to their internal user id,
// the same email/subject lookup handleListRisks' Action Owner list scoping
// uses. Returns (nil, nil) — not an error — when the caller has no platform
// user row, so callers can fail closed on that case themselves.
func (d *Deps) callerUserID(ctx context.Context) (*int, error) {
	userInfo := auth.FromContext(ctx)
	if userInfo == nil {
		return nil, nil
	}
	email := userInfo.Email
	if email == "" {
		email = userInfo.Subject
	}
	caller, err := d.Users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if caller == nil {
		return nil, nil
	}
	return &caller.ID, nil
}

// riskVisibleToCaller reports whether the caller may view riskID's data — the
// by-id counterpart to handleListRisks' list scoping, closing the gap where
// that scoping was otherwise cosmetic (a caller restricted in the list could
// still read any risk directly by id). Any caller holding a privilege outside
// both narrower tiers below always passes; their access is already governed
// by the ViewRisks check callers perform before this.
func (d *Deps) riskVisibleToCaller(ctx context.Context, riskID int) (bool, error) {
	if isActionOwnerOnly(ctx) {
		// Passes only if they're the action_owner_id of one of the risk's
		// action plans (STANDARD or MANAGEMENT).
		callerID, err := d.callerUserID(ctx)
		if err != nil || callerID == nil {
			return false, err
		}
		plans, err := d.ActionPlan.List(ctx, riskID)
		if err != nil {
			return false, err
		}
		for _, p := range plans {
			if p.ActionOwnerID != nil && *p.ActionOwnerID == *callerID {
				return true, nil
			}
		}
		return false, nil
	}
	if isTeamScopedOnly(ctx) {
		// Passes only if the caller belongs to the risk's source register or
		// assignment team.
		userInfo := auth.FromContext(ctx)
		if userInfo == nil {
			return false, nil
		}
		email := userInfo.Email
		if email == "" {
			email = userInfo.Subject
		}
		caller, err := d.Users.GetByEmail(ctx, email)
		if err != nil {
			return false, err
		}
		if caller == nil || len(caller.RiskTeamIDs) == 0 {
			return false, nil
		}
		risk, err := d.Risk.GetByID(ctx, riskID)
		if err != nil {
			return false, err
		}
		for _, teamID := range caller.RiskTeamIDs {
			if teamID == risk.SourceRegisterID || teamID == risk.AssignmentTeamID {
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
}

// handleListRisks serves GET /api/v1/risks.
// Query params:
//   - statuses:        comma-separated workflow status values
//   - team_id:          comma-separated source register IDs
//   - level:            comma-separated LOW | MEDIUM | HIGH values
//   - search:           matched against risk_code and risk_title
//   - risk_type:        comma-separated NEW | UPDATED values
//   - owner_id:          comma-separated owner user IDs
//   - submitted_from/to: created_at date range (YYYY-MM-DD, inclusive)
//   - due_from/to:       implementation_date range (YYYY-MM-DD, inclusive)
//   - due_overdue:       "true" to additionally restrict to implementation_date < today
func (d *Deps) handleListRisks(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	q := r.URL.Query()

	var filter model.ListRisksFilter
	filter.Statuses = splitCSV(q.Get("statuses"))
	filter.TeamIDs = splitCSVInts(q.Get("team_id"))
	filter.Levels = splitCSV(q.Get("level"))
	filter.Search = q.Get("search")
	filter.RiskTypes = splitCSV(q.Get("risk_type"))
	filter.OwnerIDs = splitCSVInts(q.Get("owner_id"))
	filter.SubmittedFrom = q.Get("submitted_from")
	filter.SubmittedTo = q.Get("submitted_to")
	filter.DueFrom = q.Get("due_from")
	filter.DueTo = q.Get("due_to")
	filter.DueOverdueOnly = q.Get("due_overdue") == "true"

	filter.Limit = 50
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
			filter.Limit = v
		}
	}
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			filter.Offset = v
		}
	}

	// Action Owner list scoping: a caller who can only complete action steps
	// (not create/approve/escalate risks) sees just the risks where they own
	// a plan — implementing what the grc-platform-risk-action-owner role's
	// own seed-data description promises. Broader-privilege holders (Risk
	// Assigner, Risk Owner, Compliance, Management, Admin) see everything,
	// same as before; this never narrows their view.
	//
	// Fails closed: leaving ActionOwnerID unset when the caller can't be
	// resolved would hand them the entire register — the exact exposure this
	// scoping exists to prevent — so an unresolvable caller gets an error or
	// an empty page, never an unscoped one.
	if isActionOwnerOnly(r.Context()) {
		email, ok := requireUserEmail(w, r)
		if !ok {
			return
		}
		caller, err := d.Users.GetByEmail(r.Context(), email)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if caller == nil {
			// Authenticated but with no platform user row: they cannot be any
			// plan's action_owner_id, so an empty page is the truthful scoped
			// result rather than an error.
			response.WriteJSONValue(w, http.StatusOK, model.RiskListPage{
				Items:  []*model.RiskListItem{},
				Total:  0,
				Offset: filter.Offset,
				Limit:  filter.Limit,
			})
			return
		}
		filter.ActionOwnerID = &caller.ID
	}

	// Team scoping: a Risk Assigner/Risk Owner-only caller (no Compliance/
	// Management/Admin privilege) sees only risks belonging to their own risk
	// teams. Fails closed the same way the Action Owner branch above does — an
	// unresolvable caller or one with zero team memberships gets an empty page,
	// never an unscoped one.
	if isTeamScopedOnly(r.Context()) {
		email, ok := requireUserEmail(w, r)
		if !ok {
			return
		}
		caller, err := d.Users.GetByEmail(r.Context(), email)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if caller == nil || len(caller.RiskTeamIDs) == 0 {
			response.WriteJSONValue(w, http.StatusOK, model.RiskListPage{
				Items:  []*model.RiskListItem{},
				Total:  0,
				Offset: filter.Offset,
				Limit:  filter.Limit,
			})
			return
		}
		filter.ScopeTeamIDs = caller.RiskTeamIDs
	}

	page, err := d.Risk.List(r.Context(), filter)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, page)
}

// handleGetRisk serves GET /api/v1/risks/{id}.
func (d *Deps) handleGetRisk(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	visible, err := d.riskVisibleToCaller(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !visible {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}

	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, detail)
}

// handleUpdateRisk serves PUT /api/v1/risks/{id}.
// Updating any restricted field (implementation_date, email_subject, action_steps)
// on an IN_REMEDIATION risk moves it to PENDING_AMENDMENT.
func (d *Deps) handleUpdateRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.UpdateRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}

	var req model.UpdateRiskRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	if req.RiskTitle == "" {
		response.WriteError(w, http.StatusBadRequest, "risk_title is required")
		return
	}
	if req.RiskDescription == "" {
		response.WriteError(w, http.StatusBadRequest, "risk_description is required")
		return
	}
	if req.EmailSubject == "" {
		response.WriteError(w, http.StatusBadRequest, "email_subject is required")
		return
	}

	// IdentifiedByType == "" means "leave Identified By unchanged" — see the
	// COALESCE-on-empty convention this maps onto in the repository. Only
	// validate/resolve when the caller is actually setting it this request.
	if req.IdentifiedByType != "" {
		switch req.IdentifiedByType {
		case model.IdentifiedByEmployee:
			if req.IdentifiedByEmail == nil || strings.TrimSpace(*req.IdentifiedByEmail) == "" {
				response.WriteError(w, http.StatusBadRequest, "identified_by_email is required when identified_by_type is "+model.IdentifiedByEmployee)
				return
			}
			name, err := d.resolveIdentifiedByEmployee(r.Context(), *req.IdentifiedByEmail)
			if err != nil {
				response.MapServiceError(r.Context(), w, err, "Unable to verify the identifying employee. Please try again.")
				return
			}
			req.IdentifiedByName = &name
		case model.IdentifiedByExternalPerson, model.IdentifiedByTool:
			if req.IdentifiedByName == nil || strings.TrimSpace(*req.IdentifiedByName) == "" {
				response.WriteError(w, http.StatusBadRequest, "identified_by_name is required when identified_by_type is "+req.IdentifiedByType)
				return
			}
			trimmed := strings.TrimSpace(*req.IdentifiedByName)
			req.IdentifiedByName = &trimmed
		default:
			response.WriteError(w, http.StatusBadRequest, "identified_by_type must be "+model.IdentifiedByEmployee+", "+model.IdentifiedByExternalPerson+", or "+model.IdentifiedByTool)
			return
		}
	}

	if err := d.Risk.Update(r.Context(), id, req, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleOwnerApproveRisk serves POST /api/v1/risks/{id}/owner-approve.
// Handles PENDING_RISK_OWNER_APPROVAL, PENDING_AMENDMENT, and PENDING_OWNER_COMPLETION_APPROVAL.
func (d *Deps) handleOwnerApproveRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.OwnerApproveRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if err := d.Risk.OwnerApprove(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleManagementApproveRisk serves POST /api/v1/risks/{id}/management-approve.
// Transitions PENDING_MANAGEMENT_APPROVAL → PENDING_COMPLIANCE_REVIEW.
func (d *Deps) handleManagementApproveRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManagementApproveRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	callerID, err := d.callerUserID(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if err := d.Risk.ManagementApprove(r.Context(), id, by, callerID); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleApproveRisk serves POST /api/v1/risks/{id}/approve.
// Compliance approval: PENDING_COMPLIANCE_REVIEW → IN_REMEDIATION.
func (d *Deps) handleApproveRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.ComplianceApproveRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if err := d.Risk.Approve(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rejectPrivilegeFor maps a workflow status to the privilege required to reject
// at that stage. Defaults to OwnerRejectRisk for all owner-stage states.
func rejectPrivilegeFor(status string) string {
	switch status {
	case "PENDING_MANAGEMENT_APPROVAL":
		return privilege.ManagementRejectRisk
	case "PENDING_COMPLIANCE_REVIEW":
		return privilege.ComplianceRejectRisk
	default: // PENDING_RISK_OWNER_APPROVAL, PENDING_AMENDMENT, PENDING_OWNER_COMPLETION_APPROVAL
		return privilege.OwnerRejectRisk
	}
}

// handleRejectRisk serves POST /api/v1/risks/{id}/reject.
// Routes to PENDING_REVISION from any pending-approval stage; stores rejection_stage.
// The required privilege depends on which stage the risk is currently at.
func (d *Deps) handleRejectRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}

	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, rejectPrivilegeFor(detail.WorkflowStatus)) {
		return
	}
	// Rejecting at the management stage is further restricted to the risk's
	// own designated Management Approver, same as approving it — see
	// handleManagementApproveRisk.
	if detail.WorkflowStatus == model.StatusPendingManagementApproval {
		callerID, err := d.callerUserID(r.Context())
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if callerID == nil || detail.ManagementApproverID != *callerID {
			response.WriteError(w, http.StatusForbidden, "only this risk's designated Management Approver may reject it")
			return
		}
	}

	var req model.RejectRiskRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	if err := d.Risk.Reject(r.Context(), id, req, detail.WorkflowStatus, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCompleteRisk serves POST /api/v1/risks/{id}/complete.
// Transitions IN_REMEDIATION → PENDING_OWNER_COMPLETION_APPROVAL.
func (d *Deps) handleCompleteRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CompleteRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if err := d.Risk.Complete(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleResubmitRisk serves POST /api/v1/risks/{id}/resubmit.
// Transitions PENDING_REVISION → PENDING_RISK_OWNER_APPROVAL and clears rejection info.
func (d *Deps) handleResubmitRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if err := d.Risk.Resubmit(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCancelRisk serves POST /api/v1/risks/{id}/cancel.
// Soft-deletes a risk by moving it to CANCELLED. Only valid from PENDING_RISK_OWNER_APPROVAL.
func (d *Deps) handleCancelRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CancelRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if err := d.Risk.Cancel(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCloseRisk serves POST /api/v1/risks/{id}/close.
// Transitions PENDING_COMPLIANCE_CLOSURE → CLOSED.
func (d *Deps) handleCloseRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CloseRisk) {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if err := d.Risk.Close(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

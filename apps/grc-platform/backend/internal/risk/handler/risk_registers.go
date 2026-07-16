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
	if err := d.Risk.ManagementApprove(r.Context(), id, by); err != nil {
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

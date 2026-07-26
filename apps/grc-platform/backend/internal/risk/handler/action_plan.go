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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleCreateManagementActionPlan serves POST /api/v1/risks/{id}/action-plans.
// MANAGEMENT-only — see ActionPlanService.Create's comment for why STANDARD
// plans don't go through this endpoint.
func (d *Deps) handleCreateManagementActionPlan(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CreateManagementActionPlan) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	var req model.CreateActionPlanRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	plan, err := d.ActionPlan.Create(r.Context(), riskID, req, by)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, plan)
}

// handleListActionPlans serves GET /api/v1/risks/{id}/action-plans. Visible to
// anyone who can view the risk, including MANAGEMENT plans (see the design
// decision that walked back an earlier team-only view restriction) — except
// an Action-Owner-only caller, who is further scoped to risks where they own
// a plan (riskVisibleToCaller), matching handleListRisks' list scoping.
func (d *Deps) handleListActionPlans(w http.ResponseWriter, r *http.Request) {
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
	plans, err := d.ActionPlan.List(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, plans)
}

// handleListActionPlanSteps serves GET /api/v1/risks/{id}/action-plans/{planId}/steps.
// For an Action-Owner-only caller, scoping is on the plan itself (not just "do
// they own some plan under this risk") — otherwise an owner of one plan under
// a risk could substitute a different plan id under the same risk to read
// steps that aren't theirs.
func (d *Deps) handleListActionPlanSteps(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "planId must be a positive integer")
		return
	}
	if isActionOwnerOnly(r.Context()) {
		plan, err := d.ActionPlan.GetByID(r.Context(), riskID, planID)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		callerID, err := d.callerUserID(r.Context())
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if callerID == nil || plan.ActionOwnerID == nil || *plan.ActionOwnerID != *callerID {
			response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
			return
		}
	}
	steps, err := d.ActionPlan.ListSteps(r.Context(), planID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, steps)
}

// handleUpdateActionPlanStep serves
// PATCH /api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}. This is how
// an Action Owner marks a step complete — applies uniformly to STANDARD and
// MANAGEMENT plans. Gated by CompleteActionSteps plus the service-layer
// ownership check (caller must be the plan's action_owner_id).
func (d *Deps) handleUpdateActionPlanStep(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CompleteActionSteps) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "planId must be a positive integer")
		return
	}
	stepID, err := strconv.Atoi(r.PathValue("stepId"))
	if err != nil || stepID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "stepId must be a positive integer")
		return
	}
	var req model.UpdateActionPlanStepRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if err := d.ActionPlan.UpdateStep(r.Context(), riskID, planID, stepID, req, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCompleteActionPlan serves
// POST /api/v1/risks/{id}/action-plans/{planId}/complete. Requires every step
// already COMPLETED (enforced entity-side); for a MANAGEMENT plan this also
// resolves its escalation and reverts the risk to IN_REMEDIATION.
func (d *Deps) handleCompleteActionPlan(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CompleteActionSteps) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "planId must be a positive integer")
		return
	}
	plan, err := d.ActionPlan.Complete(r.Context(), riskID, planID, by)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, plan)
}

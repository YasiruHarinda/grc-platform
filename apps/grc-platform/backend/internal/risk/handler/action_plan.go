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
	userInfo := auth.FromContext(r.Context())
	if userInfo == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CreateManagementActionPlan) {
		return
	}
	riskID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || riskID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}
	var req model.CreateActionPlanRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	plan, err := d.ActionPlan.Create(r.Context(), riskID, req, userInfo.Email)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, plan)
}

// handleListActionPlans serves GET /api/v1/risks/{id}/action-plans. Visible to
// anyone who can view the risk — no extra gating on plan content, including
// MANAGEMENT plans (see the design decision that walked back an earlier
// team-only view restriction).
func (d *Deps) handleListActionPlans(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	riskID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || riskID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
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
func (d *Deps) handleListActionPlanSteps(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "planId must be a positive integer")
		return
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
	userInfo := auth.FromContext(r.Context())
	if userInfo == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CompleteActionSteps) {
		return
	}
	riskID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || riskID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
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
	if err := d.ActionPlan.UpdateStep(r.Context(), riskID, planID, stepID, req, userInfo.Email); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]bool{"success": true})
}

// handleCompleteActionPlan serves
// POST /api/v1/risks/{id}/action-plans/{planId}/complete. Requires every step
// already COMPLETED (enforced entity-side); for a MANAGEMENT plan this also
// resolves its escalation and reverts the risk to IN_REMEDIATION.
func (d *Deps) handleCompleteActionPlan(w http.ResponseWriter, r *http.Request) {
	userInfo := auth.FromContext(r.Context())
	if userInfo == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CompleteActionSteps) {
		return
	}
	riskID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || riskID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "planId must be a positive integer")
		return
	}
	plan, err := d.ActionPlan.Complete(r.Context(), riskID, planID, userInfo.Email)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, plan)
}

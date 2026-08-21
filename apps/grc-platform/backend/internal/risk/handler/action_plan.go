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
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleCreateActionPlan serves POST /api/v1/risks/{id}/action-plans, adding a
// further STANDARD plan to a risk that already has one from registration.
//
// This endpoint used to create MANAGEMENT plans, which were how an escalation
// was answered. Escalations are now answered with a comment, so the plan type
// is gone and remediation planning belongs to the Risk Assigner: the gate moved
// from RISK_CREATE_MANAGEMENT_ACTION_PLAN (a Management privilege) to
// RISK_MANAGE_ACTION_PLANS plus the assigner identity check, matching every
// other assigner-side action.
func (d *Deps) handleCreateActionPlan(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if !d.requireRiskAssigner(w, r, riskID, privilege.ManageActionPlans) {
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
	d.recordEvent(r.Context(), riskID, by, model.HistoryCreate, model.HistoryDetails{
		Plan: planLabel(plan),
	})
	response.WriteJSONValue(w, http.StatusCreated, plan)
}

// handleListActionPlans serves GET /api/v1/risks/{id}/action-plans. Visible to
// anyone who can view the risk (see the design decision that walked back an
// earlier team-only view restriction) — except
// an Action-Owner-only caller, who is further scoped to risks where they own
// a plan (riskVisibleToCaller), matching handleListRisks' list scoping.
func (d *Deps) handleListActionPlans(w http.ResponseWriter, r *http.Request) {
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
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "planId must be a positive integer")
		return
	}
	if holdsNoGrants(r.Context()) {
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
// PATCH /api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}. This is how an
// Action Owner marks a step complete.
//
// Authorised by the identity axis alone: the caller must be the plan's
// action_owner_id, enforced service-side by requireOwner. There is deliberately
// no privilege guard — RISK_COMPLETE_ACTION_STEPS was retired with the
// action-owner role, because an Action Owner may be any employee and need hold
// no role at all. Gating on a privilege nobody can hold would 403 the very
// person the action exists for.
func (d *Deps) handleUpdateActionPlanStep(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
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
	registerID, err := d.sourceRegisterOf(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if err := d.ActionPlan.UpdateStep(r.Context(), riskID, planID, stepID, req, by,
		canOverrideAssigneeIn(r.Context(), registerID)); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCompleteActionPlan serves
// POST /api/v1/risks/{id}/action-plans/{planId}/complete. Requires every step
// already COMPLETED (enforced entity-side).
func (d *Deps) handleCompleteActionPlan(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
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
	registerID, err := d.sourceRegisterOf(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	plan, err := d.ActionPlan.Complete(r.Context(), riskID, planID, by, canOverrideAssigneeIn(r.Context(), registerID))
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), riskID, by, model.HistoryComplete, model.HistoryDetails{
		Plan: planLabel(plan),
	})
	// Fires per plan — a risk with several plans therefore sends several. This
	// is the only channel now: the in-app REASSESSMENT notification the entity
	// used to write on this same cascade went with the risk_notification table.
	d.notifyAssignerOfPlanCompletion(r.Context(), riskID, by)
	response.WriteJSONValue(w, http.StatusOK, plan)
}

// notifyAssignerOfPlanCompletion tells the risk's assigner that an action plan
// has finished and the risk is ready to be reassessed and submitted.
func (d *Deps) notifyAssignerOfPlanCompletion(ctx context.Context, riskID int, by string) {
	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("risk notification: failed to load risk for plan completion", "riskId", riskID, "err", err)
		return
	}
	d.notifyRiskEvent(emailer.EventActionPlanCompleted, riskID, []int{detail.AssignerID}, by, "")
}

// planLabel names an action plan for the history, falling back to its id when
// it has no description — a plan is optional-description, and "plan 7" reads
// better in a timeline than an empty string.
func planLabel(plan *model.ActionPlan) string {
	if plan == nil {
		return ""
	}
	if plan.Description != nil && strings.TrimSpace(*plan.Description) != "" {
		return strings.TrimSpace(*plan.Description)
	}
	return fmt.Sprintf("plan %d", plan.ID)
}

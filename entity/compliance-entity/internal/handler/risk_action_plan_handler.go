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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// RiskActionPlanHandler handles risk action plan routes.
type RiskActionPlanHandler struct{ svc service.RiskActionPlanService }

// NewRiskActionPlanHandler constructs a RiskActionPlanHandler.
func NewRiskActionPlanHandler(svc service.RiskActionPlanService) *RiskActionPlanHandler {
	return &RiskActionPlanHandler{svc: svc}
}

// CreateRiskActionPlan handles POST /risks/{riskId}/action-plans.
func (h *RiskActionPlanHandler) CreateRiskActionPlan(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil || riskID <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	var req domain.CreateRiskActionPlanRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	p, err := h.svc.CreateRiskActionPlan(r.Context(), riskID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

// ListRiskActionPlans handles GET /risks/{riskId}/action-plans.
func (h *RiskActionPlanHandler) ListRiskActionPlans(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil || riskID <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListRiskActionPlans(r.Context(), riskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetRiskActionPlanByID handles GET /risks/action-plans/{planId}.
func (h *RiskActionPlanHandler) GetRiskActionPlanByID(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	p, err := h.svc.GetRiskActionPlanByID(r.Context(), planID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// UpdateRiskActionPlan handles PATCH /risks/action-plans/{planId}.
func (h *RiskActionPlanHandler) UpdateRiskActionPlan(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	var req domain.UpdateRiskActionPlanRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	p, err := h.svc.UpdateRiskActionPlan(r.Context(), planID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// CompleteRiskActionPlan handles POST /action-plans/{planId}/complete.
func (h *RiskActionPlanHandler) CompleteRiskActionPlan(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(r.PathValue("planId"))
	if err != nil || planID <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "planId must be a positive integer"})
		return
	}
	var req domain.CompleteRiskActionPlanRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	p, err := h.svc.CompleteRiskActionPlan(r.Context(), planID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

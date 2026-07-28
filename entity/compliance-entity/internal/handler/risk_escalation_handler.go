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

// RiskEscalationHandler handles /risks/{riskId}/escalations routes.
type RiskEscalationHandler struct{ svc service.RiskEscalationService }

// NewRiskEscalationHandler constructs a RiskEscalationHandler.
func NewRiskEscalationHandler(svc service.RiskEscalationService) *RiskEscalationHandler {
	return &RiskEscalationHandler{svc: svc}
}

// CreateRiskEscalation handles POST /risks/{riskId}/escalations.
func (h *RiskEscalationHandler) CreateRiskEscalation(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	var req domain.CreateRiskEscalationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.CreateRiskEscalation(r.Context(), riskID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

// ListRiskEscalations handles GET /risks/{riskId}/escalations.
func (h *RiskEscalationHandler) ListRiskEscalations(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListRiskEscalations(r.Context(), riskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// EscalateRisk handles POST /risks/{riskId}/escalate — the manual trigger.
func (h *RiskEscalationHandler) EscalateRisk(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	var req domain.EscalateRiskRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.EscalateRisk(r.Context(), riskID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

// GetRiskEscalationByID handles GET /risks/{riskId}/escalations/{escalationId}.
func (h *RiskEscalationHandler) GetRiskEscalationByID(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	escalationID, err := strconv.Atoi(r.PathValue("escalationId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "escalationId must be a positive integer"})
		return
	}
	e, err := h.svc.GetRiskEscalationByID(r.Context(), riskID, escalationID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

// UpdateRiskEscalation handles PATCH /risks/{riskId}/escalations/{escalationId}.
func (h *RiskEscalationHandler) UpdateRiskEscalation(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	escalationID, err := strconv.Atoi(r.PathValue("escalationId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "escalationId must be a positive integer"})
		return
	}
	var req domain.UpdateRiskEscalationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.UpdateRiskEscalation(r.Context(), riskID, escalationID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

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

// ControlHandler handles /audits/{auditId}/controls routes.
type ControlHandler struct{ svc service.ControlService }

// NewControlHandler constructs a ControlHandler.
func NewControlHandler(svc service.ControlService) *ControlHandler { return &ControlHandler{svc: svc} }

// SearchControls handles POST /audits/{auditId}/controls/search.
func (h *ControlHandler) SearchControls(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	var req domain.SearchControlsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchControls(r.Context(), auditID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// SearchControlsGlobal handles POST /controls/search — cross-audit search by auditor, owner, team, or status.
func (h *ControlHandler) SearchControlsGlobal(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchControlsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchControlsGlobal(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetEvidenceAssignment handles GET /audit-controls/{controlId}/evidence-assignment?email=.
// 200 {"auditId":N} when the user is assigned to this actionable control; 404 otherwise.
func (h *ControlHandler) GetEvidenceAssignment(w http.ResponseWriter, r *http.Request) {
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	resp, err := h.svc.GetEvidenceAssignment(r.Context(), r.URL.Query().Get("email"), controlID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetActivePopulation handles GET /audit-controls/{controlId}/active-population.
// 200 {"populationId":N} for an OE control with an active round; 404 otherwise.
func (h *ControlHandler) GetActivePopulation(w http.ResponseWriter, r *http.Request) {
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	resp, err := h.svc.FindActivePopulation(r.Context(), controlID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetControlByID handles GET /audits/{auditId}/controls/{controlId}.
func (h *ControlHandler) GetControlByID(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	c, err := h.svc.GetControlByID(r.Context(), auditID, controlID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}

// CreateControl handles POST /audits/{auditId}/controls.
func (h *ControlHandler) CreateControl(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	var req domain.CreateControlRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	c, err := h.svc.CreateControl(r.Context(), auditID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(c)
}

// BulkCreateControls handles POST /audits/{auditId}/controls/bulk.
func (h *ControlHandler) BulkCreateControls(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	var req domain.BulkCreateControlsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.BulkCreateControls(r.Context(), auditID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteControl handles DELETE /audits/{auditId}/controls/{controlId}.
func (h *ControlHandler) DeleteControl(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	if err := h.svc.DeleteControl(r.Context(), auditID, controlID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateControl handles PATCH /audits/{auditId}/controls/{controlId}.
func (h *ControlHandler) UpdateControl(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	var req domain.UpdateControlRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	c, err := h.svc.UpdateControl(r.Context(), auditID, controlID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}

// OverrideControlStatus handles POST /audits/{auditId}/controls/{controlId}/status-override.
func (h *ControlHandler) OverrideControlStatus(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	var req domain.OverrideControlStatusRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	c, err := h.svc.OverrideControlStatus(r.Context(), auditID, controlID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}

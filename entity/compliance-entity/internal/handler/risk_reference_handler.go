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

// RiskReferenceHandler handles /risk/compliance-references routes.
type RiskReferenceHandler struct{ svc service.RiskReferenceService }

// NewRiskReferenceHandler constructs a RiskReferenceHandler.
func NewRiskReferenceHandler(svc service.RiskReferenceService) *RiskReferenceHandler {
	return &RiskReferenceHandler{svc: svc}
}

// SearchRiskReferences handles POST /risk/compliance-references/search.
func (h *RiskReferenceHandler) SearchRiskReferences(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchRiskReferencesRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchRiskReferences(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetRiskReferenceByID handles GET /risk/compliance-references/{id}.
func (h *RiskReferenceHandler) GetRiskReferenceByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	ref, err := h.svc.GetRiskReferenceByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ref)
}

// CreateRiskReference handles POST /risk/compliance-references.
func (h *RiskReferenceHandler) CreateRiskReference(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRiskReferenceRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	ref, err := h.svc.CreateRiskReference(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ref)
}

// UpdateRiskReference handles PATCH /risk/compliance-references/{id}.
func (h *RiskReferenceHandler) UpdateRiskReference(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateRiskReferenceRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	ref, err := h.svc.UpdateRiskReference(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ref)
}

// DeleteRiskReference handles DELETE /risk/compliance-references/{id}.
func (h *RiskReferenceHandler) DeleteRiskReference(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	if err := h.svc.DeleteRiskReference(r.Context(), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

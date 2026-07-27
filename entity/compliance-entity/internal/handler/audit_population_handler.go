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

// PopulationHandler handles population routes nested under audit controls.
type PopulationHandler struct{ svc service.PopulationService }

// NewPopulationHandler constructs a PopulationHandler.
func NewPopulationHandler(svc service.PopulationService) *PopulationHandler {
	return &PopulationHandler{svc: svc}
}

// CreatePopulation handles POST /audits/{auditId}/controls/{controlId}/populations.
func (h *PopulationHandler) CreatePopulation(w http.ResponseWriter, r *http.Request) {
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
	var req domain.CreatePopulationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	p, err := h.svc.CreatePopulation(r.Context(), auditID, controlID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

// ListPopulations handles GET /audits/{auditId}/controls/{controlId}/populations.
func (h *PopulationHandler) ListPopulations(w http.ResponseWriter, r *http.Request) {
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
	pops, err := h.svc.ListPopulations(r.Context(), auditID, controlID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pops)
}

// GetPopulationByID handles GET /populations/{populationId}.
func (h *PopulationHandler) GetPopulationByID(w http.ResponseWriter, r *http.Request) {
	populationID, err := strconv.Atoi(r.PathValue("populationId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "populationId must be a positive integer"})
		return
	}
	p, err := h.svc.GetPopulationByID(r.Context(), populationID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// UpdatePopulation handles PATCH /populations/{populationId}.
func (h *PopulationHandler) UpdatePopulation(w http.ResponseWriter, r *http.Request) {
	populationID, err := strconv.Atoi(r.PathValue("populationId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "populationId must be a positive integer"})
		return
	}
	var req domain.UpdatePopulationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	p, err := h.svc.UpdatePopulation(r.Context(), populationID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

// AddPopulationFile handles POST /populations/{populationId}/files.
func (h *PopulationHandler) AddPopulationFile(w http.ResponseWriter, r *http.Request) {
	populationID, err := strconv.Atoi(r.PathValue("populationId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "populationId must be a positive integer"})
		return
	}
	var req domain.CreatePopulationFileRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	f, err := h.svc.AddPopulationFile(r.Context(), populationID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(f)
}

// ListPopulationFiles handles GET /populations/{populationId}/files.
func (h *PopulationHandler) ListPopulationFiles(w http.ResponseWriter, r *http.Request) {
	populationID, err := strconv.Atoi(r.PathValue("populationId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "populationId must be a positive integer"})
		return
	}
	files, err := h.svc.ListPopulationFiles(r.Context(), populationID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(files)
}

// GetPopulationFileByID handles GET /populations/files/{fileId}.
func (h *PopulationHandler) GetPopulationFileByID(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "fileId must be a positive integer"})
		return
	}
	f, err := h.svc.GetPopulationFileByID(r.Context(), fileID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f)
}

// DeletePopulationFile handles DELETE /populations/files/{fileId}.
func (h *PopulationHandler) DeletePopulationFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "fileId must be a positive integer"})
		return
	}
	if err := h.svc.DeletePopulationFile(r.Context(), fileID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

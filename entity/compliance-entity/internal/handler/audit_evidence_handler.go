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

// EvidenceHandler handles evidence routes nested under controls.
type EvidenceHandler struct{ svc service.EvidenceService }

// NewEvidenceHandler constructs an EvidenceHandler.
func NewEvidenceHandler(svc service.EvidenceService) *EvidenceHandler {
	return &EvidenceHandler{svc: svc}
}

// CreateEvidence handles POST /audits/{auditId}/controls/{controlId}/evidence.
func (h *EvidenceHandler) CreateEvidence(w http.ResponseWriter, r *http.Request) {
	controlID, err := strconv.Atoi(r.PathValue("controlId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
		return
	}
	var req domain.CreateEvidenceRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.CreateEvidence(r.Context(), controlID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

// ListEvidenceByControl handles GET /audits/{auditId}/controls/{controlId}/evidence.
func (h *EvidenceHandler) ListEvidenceByControl(w http.ResponseWriter, r *http.Request) {
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
	resp, err := h.svc.ListEvidenceByControl(r.Context(), auditID, controlID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetEvidenceByID handles GET /evidence/{evidenceId}.
func (h *EvidenceHandler) GetEvidenceByID(w http.ResponseWriter, r *http.Request) {
	evidenceID, err := strconv.Atoi(r.PathValue("evidenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"})
		return
	}
	e, err := h.svc.GetEvidenceByID(r.Context(), evidenceID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

// UpdateEvidence handles PATCH /evidence/{evidenceId}.
func (h *EvidenceHandler) UpdateEvidence(w http.ResponseWriter, r *http.Request) {
	evidenceID, err := strconv.Atoi(r.PathValue("evidenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"})
		return
	}
	var req domain.UpdateEvidenceRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.UpdateEvidence(r.Context(), evidenceID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

// AddEvidenceFile handles POST /evidence/{evidenceId}/files.
func (h *EvidenceHandler) AddEvidenceFile(w http.ResponseWriter, r *http.Request) {
	evidenceID, err := strconv.Atoi(r.PathValue("evidenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"})
		return
	}
	var req domain.CreateEvidenceFileRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	f, err := h.svc.AddEvidenceFile(r.Context(), evidenceID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(f)
}

// ListEvidenceFiles handles GET /evidence/{evidenceId}/files.
func (h *EvidenceHandler) ListEvidenceFiles(w http.ResponseWriter, r *http.Request) {
	evidenceID, err := strconv.Atoi(r.PathValue("evidenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListEvidenceFiles(r.Context(), evidenceID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetEvidenceFileByID handles GET /evidence-files/{fileId}.
func (h *EvidenceHandler) GetEvidenceFileByID(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "fileId must be a positive integer"})
		return
	}
	f, err := h.svc.GetEvidenceFileByID(r.Context(), fileID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f)
}

// DeleteEvidenceFile handles DELETE /evidence-files/{fileId}.
func (h *EvidenceHandler) DeleteEvidenceFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "fileId must be a positive integer"})
		return
	}
	if err := h.svc.DeleteEvidenceFile(r.Context(), fileID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteEvidence handles DELETE /evidence/{evidenceId}. Removes a whole round
// (used for the fileless/attestation-only completion, which has no files to
// delete individually) — its files and AI validation logs cascade via FK.
func (h *EvidenceHandler) DeleteEvidence(w http.ResponseWriter, r *http.Request) {
	evidenceID, err := strconv.Atoi(r.PathValue("evidenceId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "evidenceId must be a positive integer"})
		return
	}
	if err := h.svc.DeleteEvidence(r.Context(), evidenceID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

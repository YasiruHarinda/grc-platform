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

// RiskEvidenceHandler handles risk evidence file routes.
type RiskEvidenceHandler struct{ svc service.RiskEvidenceService }

// NewRiskEvidenceHandler constructs a RiskEvidenceHandler.
func NewRiskEvidenceHandler(svc service.RiskEvidenceService) *RiskEvidenceHandler {
	return &RiskEvidenceHandler{svc: svc}
}

// CreateRiskEvidence handles POST /risks/{riskId}/evidence.
func (h *RiskEvidenceHandler) CreateRiskEvidence(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	var req domain.CreateRiskEvidenceRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	f, err := h.svc.CreateRiskEvidence(r.Context(), riskID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(f)
}

// GetRiskEvidence handles GET /risk-evidence/{fileId}. Flat by fileId (not
// nested under riskId) — mirrors the Audit Hub's GET /evidence-files/{fileId},
// used by the GRC Backend to check file ownership before allowing a delete.
func (h *RiskEvidenceHandler) GetRiskEvidence(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "fileId must be a positive integer"})
		return
	}
	f, err := h.svc.GetRiskEvidenceByID(r.Context(), fileID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f)
}

// ListRiskEvidence handles GET /risks/{riskId}/evidence.
func (h *RiskEvidenceHandler) ListRiskEvidence(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	resp, err := h.svc.ListRiskEvidence(r.Context(), riskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteRiskEvidence handles DELETE /risks/{riskId}/evidence/{fileId}. Scoped
// by riskId so a caller can never delete another risk's file by guessing its
// fileId — a mismatch 404s exactly like a missing file.
func (h *RiskEvidenceHandler) DeleteRiskEvidence(w http.ResponseWriter, r *http.Request) {
	riskID, err := strconv.Atoi(r.PathValue("riskId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "riskId must be a positive integer"})
		return
	}
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "fileId must be a positive integer"})
		return
	}
	if err := h.svc.DeleteRiskEvidence(r.Context(), riskID, fileID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

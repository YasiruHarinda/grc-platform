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

// RiskHandler handles /risks routes.
type RiskHandler struct{ svc service.RiskService }

// NewRiskHandler constructs a RiskHandler.
func NewRiskHandler(svc service.RiskService) *RiskHandler { return &RiskHandler{svc: svc} }

// SearchRisks handles POST /risks/search.
func (h *RiskHandler) SearchRisks(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchRisksRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchRisks(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetRiskByID handles GET /risks/{id}.
func (h *RiskHandler) GetRiskByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	risk, err := h.svc.GetRiskByID(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(risk)
}

// CreateRisk handles POST /risks.
func (h *RiskHandler) CreateRisk(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRiskRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	risk, err := h.svc.CreateRisk(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(risk)
}

// UpdateRisk handles PATCH /risks/{id}.
func (h *RiskHandler) UpdateRisk(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateRiskRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	risk, err := h.svc.UpdateRisk(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(risk)
}

// NextSequenceNumber handles GET /risks/next-sequence-number?sourceRegisterId=N.
// It previews the sequence number the next risk for that register would receive,
// for showing a risk code on a form before the risk exists. It consumes nothing.
func (h *RiskHandler) NextSequenceNumber(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("sourceRegisterId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "sourceRegisterId must be a positive integer"})
		return
	}
	resp, err := h.svc.NextSequenceNumber(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// GetRiskDetail handles GET /risks/{id}/detail — the whole risk page in one
// response, so a caller does not have to fan out and risk the parts disagreeing.
func (h *RiskHandler) GetRiskDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	d, err := h.svc.GetRiskDetail(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

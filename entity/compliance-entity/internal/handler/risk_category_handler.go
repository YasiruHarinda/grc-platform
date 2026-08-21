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

// RiskCategoryHandler handles /risk/categories routes.
type RiskCategoryHandler struct{ svc service.RiskCategoryService }

// NewRiskCategoryHandler constructs a RiskCategoryHandler.
func NewRiskCategoryHandler(svc service.RiskCategoryService) *RiskCategoryHandler {
	return &RiskCategoryHandler{svc: svc}
}

// ListRiskCategories handles GET /risk/categories.
func (h *RiskCategoryHandler) ListRiskCategories(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.ListRiskCategories(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateRiskCategory handles POST /risk/categories.
func (h *RiskCategoryHandler) CreateRiskCategory(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRiskCategoryRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	c, err := h.svc.CreateRiskCategory(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(c)
}

// UpdateRiskCategory handles PATCH /risk/categories/{id}.
func (h *RiskCategoryHandler) UpdateRiskCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	var req domain.UpdateRiskCategoryRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	c, err := h.svc.UpdateRiskCategory(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}

// DeleteRiskCategory handles DELETE /risk/categories/{id}.
func (h *RiskCategoryHandler) DeleteRiskCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "id must be a positive integer"})
		return
	}
	if err := h.svc.DeleteRiskCategory(r.Context(), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

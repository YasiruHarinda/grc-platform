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

// AuditTrailHandler handles /audits/{auditId}/trail routes.
type AuditTrailHandler struct{ svc service.AuditTrailService }

// NewAuditTrailHandler constructs a AuditTrailHandler.
func NewAuditTrailHandler(svc service.AuditTrailService) *AuditTrailHandler { return &AuditTrailHandler{svc: svc} }

// CreateAuditTrail handles POST /audits/{auditId}/trail.
func (h *AuditTrailHandler) CreateAuditTrail(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	var req domain.CreateAuditTrailRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.CreateAuditTrail(r.Context(), auditID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

// ListAuditTrail handles GET /audits/{auditId}/trail.
func (h *AuditTrailHandler) ListAuditTrail(w http.ResponseWriter, r *http.Request) {
	auditID, err := strconv.Atoi(r.PathValue("auditId"))
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "auditId must be a positive integer"})
		return
	}
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	// Optional ?controlId=<n> narrows the trail to a single control.
	var controlID *int
	if raw := r.URL.Query().Get("controlId"); raw != "" {
		cid, err := strconv.Atoi(raw)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
			return
		}
		controlID = &cid
	}
	resp, err := h.svc.ListAuditTrail(r.Context(), auditID, controlID, limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

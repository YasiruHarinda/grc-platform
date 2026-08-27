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
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// trailDateFormat is the query-param format for ?from=/?to= — date-only (no
// time), matching a typical date-range picker. ?to is treated as inclusive of
// that whole day (see below).
const trailDateFormat = "2006-01-02"

// AuditTrailHandler handles /audits/{auditId}/trail routes.
type AuditTrailHandler struct{ svc service.AuditTrailService }

// NewAuditTrailHandler constructs a AuditTrailHandler.
func NewAuditTrailHandler(svc service.AuditTrailService) *AuditTrailHandler {
	return &AuditTrailHandler{svc: svc}
}

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
	q := r.URL.Query()
	// Optional repeated ?controlId=<n>&controlId=<n>... narrows to one or more
	// controls (OR'd). Empty means "every control (and audit-level rows)".
	var filter domain.TrailFilter
	for _, raw := range q["controlId"] {
		cid, err := strconv.Atoi(raw)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "controlId must be a positive integer"})
			return
		}
		filter.ControlIDs = append(filter.ControlIDs, cid)
	}
	if raw := q.Get("from"); raw != "" {
		from, err := time.Parse(trailDateFormat, raw)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "from must be YYYY-MM-DD"})
			return
		}
		filter.From = &from
	}
	if raw := q.Get("to"); raw != "" {
		to, err := time.Parse(trailDateFormat, raw)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "to must be YYYY-MM-DD"})
			return
		}
		// Inclusive of the whole day: shift to the start of the next day.
		to = to.AddDate(0, 0, 1)
		filter.To = &to
	}
	// scope/userId/scopeTeamId row-scope control-level entries — see domain.TrailFilter.
	filter.Scope = domain.Scope(q.Get("scope"))
	if raw := q.Get("userId"); raw != "" {
		userID, err := strconv.Atoi(raw)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "userId must be a positive integer"})
			return
		}
		filter.UserID = userID
	}
	for _, raw := range q["scopeTeamId"] {
		id, err := strconv.Atoi(raw)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "scopeTeamId must be a positive integer"})
			return
		}
		filter.ScopeTeamIDs = append(filter.ScopeTeamIDs, id)
	}
	filter.IncludeInternal = q.Get("includeInternal") == "true"
	resp, err := h.svc.ListAuditTrail(r.Context(), auditID, filter, limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

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
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// AdminActivityLogHandler handles /admin-activity-log routes.
type AdminActivityLogHandler struct {
	svc service.AdminActivityLogService
}

// NewAdminActivityLogHandler constructs an AdminActivityLogHandler.
func NewAdminActivityLogHandler(svc service.AdminActivityLogService) *AdminActivityLogHandler {
	return &AdminActivityLogHandler{svc: svc}
}

// CreateAdminActivityLog handles POST /admin-activity-log.
func (h *AdminActivityLogHandler) CreateAdminActivityLog(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAdminActivityLogRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	e, err := h.svc.CreateAdminActivityLog(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

// ListAdminActivityLog handles GET /admin-activity-log.
func (h *AdminActivityLogHandler) ListAdminActivityLog(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	var filter domain.AdminActivityLogFilter
	filter.ActorID = q.Get("actorId")
	filter.Action = q.Get("action")
	filter.EntityType = q.Get("entityType")
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
	resp, err := h.svc.ListAdminActivityLog(r.Context(), filter, limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

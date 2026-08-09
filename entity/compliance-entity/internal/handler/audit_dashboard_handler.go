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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// DashboardHandler serves the audit dashboard aggregation.
type DashboardHandler struct {
	svc service.DashboardService
}

// NewDashboardHandler constructs a DashboardHandler.
func NewDashboardHandler(svc service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// GetDashboard handles POST /audit/dashboard/search.
// Body: { scope, workQueueScope, workQueueClass, userEmail }.
func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	var req domain.AuditDashboardRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	data, err := h.svc.Get(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// GetWorkQueuePage handles POST /audit/work-queue/search.
// Body: { workQueueScope, workQueueClass, userEmail, tab, page, limit, ...filters }.
func (h *DashboardHandler) GetWorkQueuePage(w http.ResponseWriter, r *http.Request) {
	var req domain.WorkQueueRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	page, err := h.svc.GetWorkQueuePage(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}

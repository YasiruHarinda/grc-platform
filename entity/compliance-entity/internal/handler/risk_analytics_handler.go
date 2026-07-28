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

// RiskAnalyticsHandler serves the assembled risk analytics page.
type RiskAnalyticsHandler struct{ svc service.RiskAnalyticsService }

// NewRiskAnalyticsHandler constructs a RiskAnalyticsHandler.
func NewRiskAnalyticsHandler(svc service.RiskAnalyticsService) *RiskAnalyticsHandler {
	return &RiskAnalyticsHandler{svc: svc}
}

// Summary handles POST /risk/analytics/search. POST rather than GET because the
// scope is a request body, matching the dashboard endpoint.
func (h *RiskAnalyticsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	var req domain.RiskAnalyticsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Summary(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

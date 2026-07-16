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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package handler

import (
	"net/http"
	"strconv"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleAnalyticsSummary serves GET /api/v1/risks/analytics/summary.
// Optional query param register_id scopes the payload to one register.
func (d *Deps) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAnalytics) {
		return
	}

	var registerID *int
	if raw := r.URL.Query().Get("register_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			response.WriteError(w, http.StatusBadRequest, "register_id must be a positive integer")
			return
		}
		registerID = &id
	}

	summary, err := d.Analytics.Summary(r.Context(), registerID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, summary)
}

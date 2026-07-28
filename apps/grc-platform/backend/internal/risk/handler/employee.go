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
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// minEmployeeSearchQueryLen avoids firing an HR entity lookup (and returning
// a huge/meaningless result) before the user has typed enough to search on.
const minEmployeeSearchQueryLen = 2

// handleSearchEmployees serves GET /api/v1/employees/search?q=.
// Looks up active WSO2 employees by email substring, live from the HR
// entity service — this data is never read from or written to the GRC
// platform's own database. On upstream failure, the caller (Add/Edit Risk
// form) is expected to show an inline error and block employee selection
// rather than fall back to free text.
//
// Gated on the same privileges as /users/resolve: every caller of this
// endpoint (the "Identified By" and "Action Owner" pickers, in both Add Risk
// and Edit Risk) only appears inside the create/update-risk flow, so there is
// no legitimate caller holding neither privilege.
func (d *Deps) handleSearchEmployees(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.CreateRisk, privilege.UpdateRisk) {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < minEmployeeSearchQueryLen {
		response.WriteJSONValue(w, http.StatusOK, []model.EmployeeOption{})
		return
	}

	employees, err := d.Employee.Search(r.Context(), q)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, "Unable to reach the employee directory. Please try again.")
		return
	}

	if employees == nil {
		employees = []model.EmployeeOption{}
	}
	response.WriteJSONValue(w, http.StatusOK, employees)
}

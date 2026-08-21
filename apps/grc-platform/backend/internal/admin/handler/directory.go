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
	"sort"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// minSearchLen keeps this from ever answering an empty/near-empty query with
// (effectively) the whole ~23-person snapshot — cheap to allow today, but the
// right shape regardless of how small the WSO2-org snapshot happens to be
// right now (see ADMIN_CONSOLE_DESIGN.md §5.1).
const minSearchLen = 2

// handleSearchDirectory serves GET /api/v1/admin/directory/search?q=. Powers
// the "Add User" typeahead — see directory.Service.SearchDomain for why this
// is a snapshot substring match rather than a live SCIM call.
func (d *Deps) handleSearchDirectory(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < minSearchLen {
		response.WriteError(w, http.StatusBadRequest, "q must be at least 2 characters")
		return
	}

	var people []directory.Person
	if d.Directory != nil {
		people = d.Directory.SearchDomain(q)
	}
	sort.Slice(people, func(i, j int) bool { return people[i].DisplayName < people[j].DisplayName })

	type result struct {
		UUID        string `json:"uuid"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	}
	out := make([]result, 0, len(people))
	for _, p := range people {
		out = append(out, result{UUID: p.UUID, Email: p.Email, DisplayName: p.DisplayName})
	}
	response.WriteJSONValue(w, http.StatusOK, out)
}

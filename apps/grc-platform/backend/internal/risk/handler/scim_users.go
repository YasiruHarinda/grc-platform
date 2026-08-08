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
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// handleListManagementApprovers serves GET /api/v1/management-approvers:
// every platform user who is also a member of the Management Asgardeo group
// (d.Groups.Management, RISK_MANAGEMENT_GROUP — see config.RiskGroupsConfig).
func (d *Deps) handleListManagementApprovers(w http.ResponseWriter, r *http.Request) {
	d.handleListUsersInGroup(w, r, d.Groups.Management)
}

// handleListRiskOwnerCandidates serves GET /api/v1/risk-owner-candidates:
// every platform user who is also a member of the Risk Owner Asgardeo group
// (d.Groups.RiskOwner, RISK_OWNER_GROUP — see config.RiskGroupsConfig). The
// Add Risk form further intersects this with team membership client-side.
func (d *Deps) handleListRiskOwnerCandidates(w http.ResponseWriter, r *http.Request) {
	d.handleListUsersInGroup(w, r, d.Groups.RiskOwner)
}

// handleListUsersInGroup returns the subset of platform users (from GET
// /api/v1/users' backing list) whose email matches a member of groupName in
// Asgardeo. A user who holds the Asgardeo group but has never been
// provisioned into this platform's `user` table (see resolve.go) simply
// won't appear yet — that gap isn't closed here.
func (d *Deps) handleListUsersInGroup(w http.ResponseWriter, r *http.Request, groupName string) {
	emails, err := d.SCIM.ListGroupMemberEmails(r.Context(), groupName)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, "Unable to reach the identity directory. Please try again.")
		return
	}
	memberEmails := make(map[string]struct{}, len(emails))
	for _, e := range emails {
		memberEmails[strings.ToLower(e)] = struct{}{}
	}

	all, err := d.Users.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	filtered := make([]*userentity.User, 0, len(all))
	for _, u := range all {
		if _, ok := memberEmails[strings.ToLower(u.Email)]; ok {
			filtered = append(filtered, u)
		}
	}
	response.WriteJSONValue(w, http.StatusOK, filtered)
}

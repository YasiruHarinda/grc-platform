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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// handleListUsers returns every active user, for the Risk module's general
// user dropdowns (e.g. the Add Risk form). Risk-only — see the doc comment on
// RegisterRoutes for why this route name reads as more "shared" than it is.
//
// A user who doesn't resolve through the identity directory is dropped from
// the list rather than shown with a blank name — the platform is removing
// stored names/emails entirely, so there is nothing left to fall back to,
// and the same "must resolve to be offered" rule already applies to every
// other picker (see candidates.go's resolveCandidates).
func handleListUsers(repo userentity.Repository, dir *directory.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := repo.List(r.Context())
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}

		out := make([]*userentity.User, 0, len(users))
		if dir != nil {
			uuids := make([]string, 0, len(users))
			for _, u := range users {
				uuids = append(uuids, u.UUID)
			}
			people := dir.LookupAll(r.Context(), uuids)
			for _, u := range users {
				p, ok := people[u.UUID]
				if !ok {
					continue
				}
				out = append(out, &userentity.User{
					ID: u.ID, UUID: u.UUID, Email: p.Email, DisplayName: p.DisplayName,
					Status: u.Status, RiskTeamIDs: u.RiskTeamIDs,
				})
			}
		}
		response.WriteJSONValue(w, http.StatusOK, out)
	}
}

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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

type myProfileResponse struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	ThumbnailURL string `json:"thumbnail_url"`
	// UserID is the caller's own internal user.id, resolved by uuid — the same
	// resolution the auth middleware already does for grants. Lets a form
	// default a picker (e.g. Add Risk's "Risk Assigned To") to the signed-in
	// user without the frontend re-deriving identity itself from a decoded ID
	// token, which doesn't exist at all in mock-auth mode. Omitted when the
	// caller has no platform user row yet.
	UserID *int `json:"user_id,omitempty"`
}

// handleGetMyProfile returns the signed-in user's name, profile photo, and
// internal user id.
//
// Name/photo come from hr_entity, looked up by email — Asgardeo's ID
// token/userinfo don't carry name/picture claims for this org's application,
// so this is the source of truth for the account menu instead. UserID comes
// from the platform's own `user` table, looked up by uuid — a fully
// independent resolution, so one lookup failing (e.g. HR entity down) doesn't
// take the other down with it.
func handleGetMyProfile(hrClient *hrentity.Client, users userentity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userInfo := auth.FromContext(r.Context())
		if userInfo == nil || userInfo.Email == "" {
			response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
			return
		}

		var resp myProfileResponse
		if u, err := users.GetByUUID(r.Context(), userInfo.Subject); err == nil && u != nil {
			resp.UserID = &u.ID
		}

		emp, err := hrClient.GetEmployeeByEmail(r.Context(), userInfo.Email)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if emp == nil {
			response.WriteJSONValue(w, http.StatusOK, resp)
			return
		}

		resp.FirstName = emp.FirstName
		resp.LastName = emp.LastName
		resp.ThumbnailURL = emp.Thumbnail
		response.WriteJSONValue(w, http.StatusOK, resp)
	}
}

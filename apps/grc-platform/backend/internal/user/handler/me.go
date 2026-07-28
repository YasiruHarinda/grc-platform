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
)

type myProfileResponse struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// handleGetMyProfile returns the signed-in user's name and profile photo,
// looked up from hr_entity by their own email — Asgardeo's ID token/userinfo
// don't carry name/picture claims for this org's application, so this is
// the source of truth for the account menu instead.
func handleGetMyProfile(hrClient *hrentity.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userInfo := auth.FromContext(r.Context())
		if userInfo == nil || userInfo.Email == "" {
			response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
			return
		}

		emp, err := hrClient.GetEmployeeByEmail(r.Context(), userInfo.Email)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if emp == nil {
			response.WriteJSONValue(w, http.StatusOK, myProfileResponse{})
			return
		}

		response.WriteJSONValue(w, http.StatusOK, myProfileResponse{
			FirstName:    emp.FirstName,
			LastName:     emp.LastName,
			ThumbnailURL: emp.Thumbnail,
		})
	}
}

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

	"github.com/wso2-open-operations/grc-platform/backend/internal/response"
	userentity "github.com/wso2-open-operations/grc-platform/backend/internal/user"
)

// handleListUsers returns all active users. Used by both Risk and Audit form dropdowns.
func handleListUsers(repo userentity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := repo.List(r.Context())
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		response.WriteJSONValue(w, http.StatusOK, users)
	}
}

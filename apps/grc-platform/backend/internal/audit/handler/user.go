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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type userHandler struct {
	svc       service.UserService
	directory *directory.Service
}

// listUsers handles GET /api/v1/audit/users.
// Returns all active users for owner/auditor assignment dropdowns. The user
// table stores no display name or email (see model.UserRef.UUID), so those
// are resolved in bulk via the identity directory before responding.
func (h *userHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	users, err := h.svc.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if users == nil {
		users = []*model.UserRef{}
	}

	uuidTypes := make(map[string]string, len(users))
	for _, u := range users {
		if u.UUID != "" {
			uuidTypes[u.UUID] = u.UserType
		}
	}
	people := h.directory.LookupAllTyped(r.Context(), uuidTypes)
	for _, u := range users {
		if p, ok := people[u.UUID]; ok {
			u.DisplayName = p.DisplayName
			u.Email = p.Email
		}
	}

	response.WriteJSONValue(w, http.StatusOK, users)
}

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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

type resolveUserRequest struct {
	Email string `json:"email"`
}

// handleResolveUser links an HR entity employee (identified by email, as
// returned by GET /api/v1/employees/search) to an internal user.id, creating
// the user row on the fly if one doesn't exist yet. Used wherever a form
// needs to assign any employee — not just an existing grc-platform
// user — to an FK field (e.g. a risk's Action Owner).
//
// The display name is always looked up from hr_entity here, never taken from
// the request body. Upsert's write is `ON DUPLICATE KEY UPDATE display_name
// = VALUES(display_name)` — it overwrites the row unconditionally when the
// email already exists — so a client-supplied name would let any caller
// holding CreateRisk/UpdateRisk rename an arbitrary existing platform user
// (their real email is often guessable) rather than only provisioning new
// ones. Rejecting an email hr_entity doesn't recognise closes the same gap
// for newly-created rows.
//
// This writes, so it is gated. The privileges are the risk module's because
// that is the only flow that calls it: an employee is resolved to a user id
// while creating or editing a risk. Either privilege is enough — a user who
// may only edit still has to assign an action owner.
func handleResolveUser(repo userentity.Repository, hrClient *hrentity.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.RequireAnyPrivilege(r.Context(), w, privilege.CreateRisk, privilege.UpdateRisk) {
			return
		}

		var req resolveUserRequest
		if err := response.DecodeJSON(w, r, &req); err != nil {
			return
		}

		req.Email = strings.TrimSpace(req.Email)
		if req.Email == "" {
			response.WriteError(w, http.StatusBadRequest, "email is required")
			return
		}

		emp, err := hrClient.GetEmployeeByEmail(r.Context(), req.Email)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, "Unable to reach the employee directory. Please try again.")
			return
		}
		if emp == nil {
			response.WriteError(w, http.StatusUnprocessableEntity, "email does not match an active WSO2 employee")
			return
		}
		displayName := strings.TrimSpace(strings.TrimSpace(emp.FirstName) + " " + strings.TrimSpace(emp.LastName))
		if displayName == "" {
			response.WriteError(w, http.StatusUnprocessableEntity, "email does not match an active WSO2 employee")
			return
		}

		// Attribution for the provisioned row: the Compliance Entity records
		// this as created_by/updated_by (see auth.FromContext — nil only when
		// the Auth middleware didn't run).
		var actorEmail string
		if info := auth.FromContext(r.Context()); info != nil {
			actorEmail = info.Email
		}

		u, err := repo.Upsert(r.Context(), req.Email, displayName, actorEmail)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		response.WriteJSONValue(w, http.StatusOK, u)
	}
}

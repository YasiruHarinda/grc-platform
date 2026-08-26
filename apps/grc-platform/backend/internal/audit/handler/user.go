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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type userHandler struct {
	svc       service.UserService
	directory *directory.Service
	// grants resolves which users hold AUDIT_SELECT_SAMPLE for
	// listAuditorCandidates. Nil in local dev (no privilege store
	// configured) — that path falls back to every EXTERNAL user instead.
	grants grant.Repository
}

// listUsers handles GET /api/v1/audit/users.
// Returns all active users for owner/auditor assignment dropdowns. The user
// table stores no display name or email (see model.UserRef.UUID), so those
// are resolved in bulk via the identity directory before responding.
//
// Internal-only: dumps the full user directory, which external auditors
// don't need. Gated on ViewInternalComments, the same internal-vs-external
// signal comment.go already uses — external-auditor holds ViewAudits but not it.
func (h *userHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewInternalComments) {
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

// listAuditorCandidates handles GET /api/v1/audit/auditor-candidates.
// Returns all users who hold AUDIT_SELECT_SAMPLE (INTERNAL or EXTERNAL —
// the role is assignable to either) for the Auditor POC picker in Create
// Audit / Manage Controls.
func (h *userHandler) listAuditorCandidates(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	users, err := h.svc.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	var allowed map[int]bool
	// Local dev (no privilege store configured): no grants to query, so
	// every user is offered rather than silently emptying the picker.
	if h.grants != nil && !auth.AllowAll(r.Context()) {
		candidates, err := h.grants.Candidates(r.Context(), privilege.SelectSample, nil)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		allowed = make(map[int]bool, len(candidates))
		for _, c := range candidates {
			allowed[c.ID] = true
		}
	}

	filtered := make([]*model.UserRef, 0, len(users))
	uuidTypes := make(map[string]string, len(users))
	for _, u := range users {
		if allowed != nil && !allowed[u.ID] {
			continue
		}
		filtered = append(filtered, u)
		if u.UUID != "" {
			uuidTypes[u.UUID] = u.UserType
		}
	}
	people := h.directory.LookupAllTyped(r.Context(), uuidTypes)
	for _, u := range filtered {
		if p, ok := people[u.UUID]; ok {
			u.DisplayName = p.DisplayName
			u.Email = p.Email
		}
	}

	response.WriteJSONValue(w, http.StatusOK, filtered)
}

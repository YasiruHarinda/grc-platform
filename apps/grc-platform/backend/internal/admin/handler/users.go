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
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// actor returns the caller's uuid for created_by/revoked_by attribution — the
// uuid, not their email, per the identity migration (see resolve.go's actor
// comment for the fuller reasoning). Empty when unauthenticated, which cannot
// actually reach here past the MANAGE_USERS gate in a real deployment, but
// costs nothing to guard defensively.
func actor(r *http.Request) string {
	if info := auth.FromContext(r.Context()); info != nil {
		return info.Subject
	}
	return ""
}

// handleListUsers serves GET /api/v1/admin/users?q=. q is accepted for
// backward compatibility but currently unused: the entity has no
// name/email left to filter on server-side since the uuid-identity
// migration, so name/email filtering happens client-side instead (see
// SearchUsers's doc comment).
func (d *Deps) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}

	users, err := d.Admin.SearchUsers(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if users == nil {
		users = []admin.User{}
	}
	fillNamesFromDirectory(r.Context(), d.Directory, users)
	response.WriteJSONValue(w, http.StatusOK, users)
}

// fillNamesFromDirectory resolves display name/email for rows the entity has
// neither for — every user provisioned through this console going forward,
// since Add User stores uuid only (see the uuid-identity migration). Reading
// them back from the identity directory's cache at list time, rather than
// storing them, is the whole point of that migration: the row stays uuid-only
// in the database, and the name/email shown here are always current, not a
// snapshot from whenever the person was added.
//
// A no-op per row whose entity data is already populated (pre-migration rows,
// or anyone provisioned via the email-keyed Action Owner flow) — those are
// left exactly as the entity returned them. d.Directory being nil (SCIM not
// configured, local dev) degrades to today's blank display rather than
// panicking.
func fillNamesFromDirectory(ctx context.Context, dir *directory.Service, users []admin.User) {
	if dir == nil {
		return
	}
	var uuids []string
	for _, u := range users {
		if (u.DisplayName == "" || u.Email == "") && u.UUID != "" {
			uuids = append(uuids, u.UUID)
		}
	}
	if len(uuids) == 0 {
		return
	}
	people := dir.LookupAll(ctx, uuids)
	for i := range users {
		p, ok := people[users[i].UUID]
		if !ok {
			continue
		}
		if users[i].DisplayName == "" {
			users[i].DisplayName = p.DisplayName
		}
		if users[i].Email == "" {
			users[i].Email = p.Email
		}
	}
}

type createUserRequest struct {
	UUID string `json:"uuid"`
}

// createUserResponse mirrors just enough of the entity's User for the "Add
// User" flow's success toast — the display name specifically comes from the
// directory lookup below, not from the created row (which stores none — see
// the uuid-identity migration), so it has to be assembled here rather than
// passed through from d.Users.Upsert's return value.
type createUserResponse struct {
	ID          int    `json:"id"`
	UUID        string `json:"uuid"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// handleCreateUser serves POST /api/v1/admin/users, body {"uuid": "..."}.
//
// Provisions by uuid alone: "available in the WSO2 organization" is checked
// against the identity directory, which is also where the created row's
// display name/email are sourced for the response — nothing is stored beyond
// the uuid itself (see internal/shared/entity's CreateUserRequest.Email
// comment for why). This is stricter than the Risk module's Action Owner
// resolve flow, which accepts an unprovisioned email: this endpoint is
// granting platform authority to someone, so a directory match is required,
// not merely accepted when available.
func (d *Deps) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}

	var req createUserRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	req.UUID = strings.TrimSpace(req.UUID)
	if req.UUID == "" {
		response.WriteError(w, http.StatusBadRequest, "uuid is required")
		return
	}

	if d.Directory == nil {
		response.WriteError(w, http.StatusUnprocessableEntity, "identity directory is not configured")
		return
	}
	person, found := d.Directory.Lookup(r.Context(), req.UUID)
	if !found {
		response.WriteError(w, http.StatusUnprocessableEntity, "uuid does not match a WSO2-org account")
		return
	}

	createdBy := actor(r)
	u, err := d.Users.Upsert(r.Context(), req.UUID, createdBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	response.WriteJSONValue(w, http.StatusCreated, createUserResponse{
		ID: u.ID, UUID: u.UUID, DisplayName: person.DisplayName, Email: person.Email,
	})
}

type createGrantRequest struct {
	RoleID    int    `json:"roleId"`
	ScopeType string `json:"scopeType"`
	ScopeID   int    `json:"scopeId"`
}

// handleCreateGrant serves POST /api/v1/admin/users/{id}/grants.
func (d *Deps) handleCreateGrant(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}
	if d.Grants == nil {
		response.WriteError(w, http.StatusInternalServerError, response.ErrMsgInternal)
		return
	}

	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	var req createGrantRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	g, err := d.Grants.CreateGrant(r.Context(), userID, grant.CreateGrantRequest{
		RoleID: req.RoleID, ScopeType: req.ScopeType, ScopeID: req.ScopeID, CreatedBy: actor(r),
	})
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, g)
}

// handleRevokeGrant serves DELETE /api/v1/admin/users/{id}/grants/{grantId}.
//
// revokedBy is the authenticated caller, never a client-supplied value —
// unlike the entity's own DELETE endpoint (which takes ?revokedBy= because it
// trusts whichever GRC-backend caller already authenticated the request),
// letting the client name who gets attributed here would let a compromised
// or buggy frontend record a revocation as someone else's action.
func (d *Deps) handleRevokeGrant(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}
	if d.Grants == nil {
		response.WriteError(w, http.StatusInternalServerError, response.ErrMsgInternal)
		return
	}

	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}
	grantID, err := strconv.Atoi(r.PathValue("grantId"))
	if err != nil || grantID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "grantId must be a positive integer")
		return
	}

	if err := d.Grants.RevokeGrant(r.Context(), userID, grantID, actor(r)); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

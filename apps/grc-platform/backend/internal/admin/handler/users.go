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
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
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

// handleListUsers serves GET /api/v1/admin/users. There is no server-side
// name/email filter: the entity has no name/email left to filter on since
// the uuid-identity migration, so name/email filtering happens client-side
// instead (see SearchUsers's doc comment).
func (d *Deps) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}

	users, err := d.Admin.SearchUsers(r.Context())
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
	uuidTypes := make(map[string]string)
	for _, u := range users {
		if (u.DisplayName == "" || u.Email == "") && u.UUID != "" {
			uuidTypes[u.UUID] = u.UserType
		}
	}
	if len(uuidTypes) == 0 {
		return
	}
	people := dir.LookupAllTyped(ctx, uuidTypes)
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
	// UserType — INTERNAL or EXTERNAL. Empty defaults to INTERNAL (the
	// entity's own default), matching every caller before External
	// provisioning existed.
	UserType string `json:"userType"`
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

// validUserTypes mirrors user.user_type's enum — the only two values this
// endpoint (or the column) ever accepts.
var validUserTypes = map[string]bool{"": true, "INTERNAL": true, "EXTERNAL": true}

// handleCreateUser serves POST /api/v1/admin/users, body
// {"uuid": "...", "userType": "INTERNAL" | "EXTERNAL"}.
//
// Provisions by uuid alone: "available in the WSO2 organization" (or, for
// EXTERNAL, the external auditor organization) is checked against the
// identity directory, which is also where the created row's display
// name/email are sourced for the response — nothing is stored beyond the
// uuid itself (see internal/shared/entity's CreateUserRequest.Email comment
// for why). This is stricter than the Risk module's Action Owner resolve
// flow, which accepts an unprovisioned email: this endpoint is granting
// platform authority to someone, so a directory match is required, not
// merely accepted when available.
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
	req.UserType = strings.ToUpper(strings.TrimSpace(req.UserType))
	if !validUserTypes[req.UserType] {
		response.WriteError(w, http.StatusBadRequest, "userType must be INTERNAL or EXTERNAL")
		return
	}

	if d.Directory == nil {
		response.WriteError(w, http.StatusUnprocessableEntity, "identity directory is not configured")
		return
	}
	// LookupTyped treats anything but the literal "EXTERNAL" as INTERNAL,
	// matching the empty-string-defaults-to-INTERNAL contract validated
	// above.
	person, found := d.Directory.LookupTyped(r.Context(), req.UUID, req.UserType)
	if !found {
		response.WriteError(w, http.StatusUnprocessableEntity, "uuid does not match a WSO2-org account")
		return
	}

	createdBy := actor(r)
	u, err := d.Users.UpsertTyped(r.Context(), req.UUID, req.UserType, createdBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	userLabel := u.UUID
	if strings.TrimSpace(person.DisplayName) != "" {
		userLabel = strings.TrimSpace(person.DisplayName)
	} else if person.Email != "" {
		userLabel = person.Email
	}
	d.ActivityLog.Log(r.Context(), createdBy, adminactivity.ActionCreated, adminactivity.EntityUser, u.ID,
		map[string]any{"user": userLabel, "userType": req.UserType})

	response.WriteJSONValue(w, http.StatusCreated, createUserResponse{
		ID: u.ID, UUID: u.UUID, DisplayName: person.DisplayName, Email: person.Email,
	})
}

// validUserStatuses mirrors user.status's enum.
var validUserStatuses = map[string]bool{"ACTIVE": true, "INACTIVE": true, "REMOVED": true}

type updateUserStatusRequest struct {
	Status string `json:"status"`
}

// handleUpdateUserStatus serves PATCH /api/v1/admin/users/{id}/status.
//
// Self-lockout guard: a caller may not change their own status, since that
// would zero all of their privileges outright — always, for any status.
// handleRevokeGrant has a narrower cousin: it blocks only a self-revoke of the
// SHARED/GLOBAL grant that carries MANAGE_USERS.
func (d *Deps) handleUpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}

	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	if caller := auth.FromContext(r.Context()); caller != nil && caller.UserID == userID {
		response.WriteError(w, http.StatusUnprocessableEntity,
			"you cannot change your own status — ask another admin to do it")
		return
	}

	var req updateUserStatusRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if !validUserStatuses[req.Status] {
		response.WriteError(w, http.StatusBadRequest, "status must be ACTIVE, INACTIVE, or REMOVED")
		return
	}

	callerUUID := actor(r)
	u, err := d.Users.UpdateStatus(r.Context(), userID, req.Status, callerUUID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if u == nil {
		response.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	d.ActivityLog.Log(r.Context(), callerUUID, adminactivity.ActionStatusChanged, adminactivity.EntityUser, userID,
		map[string]any{"user": resolveUserLabel(r.Context(), d, userID), "status": req.Status})
	w.WriteHeader(http.StatusNoContent)
}

type createGrantRequest struct {
	RoleID    int    `json:"roleId"`
	ScopeType string `json:"scopeType"`
	ScopeID   int    `json:"scopeId"`
}

// grantScopeLabel renders a grant's scope for the activity log — a resolved
// team name reads better than a bare scope_id an admin has to go look up.
func grantScopeLabel(scopeType, scopeName string) string {
	if scopeType == "GLOBAL" {
		return "Global (ALL)"
	}
	if scopeName != "" {
		return scopeName
	}
	return scopeType
}

// resolveUserLabel resolves a userID to a display name, falling back to
// email then the bare uuid so a grant entry always names who it affected.
func resolveUserLabel(ctx context.Context, d *Deps, userID int) string {
	u, err := d.Users.GetByID(ctx, userID)
	if err != nil || u == nil || u.UUID == "" {
		return ""
	}
	if d.Directory != nil {
		if p, ok := d.Directory.LookupTyped(ctx, u.UUID, ""); ok {
			if name := strings.TrimSpace(p.DisplayName); name != "" {
				return name
			}
			if p.Email != "" {
				return p.Email
			}
		}
	}
	return u.UUID
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

	createdBy := actor(r)
	g, err := d.Grants.CreateGrant(r.Context(), userID, grant.CreateGrantRequest{
		RoleID: req.RoleID, ScopeType: req.ScopeType, ScopeID: req.ScopeID, CreatedBy: createdBy,
	})
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// grant.Grant carries no id of its own — entityId is the affected user.
	grantDetails := map[string]any{"role": g.RoleName, "scope": grantScopeLabel(req.ScopeType, g.ScopeName)}
	if label := resolveUserLabel(r.Context(), d, userID); label != "" {
		grantDetails["user"] = label
	}
	d.ActivityLog.Log(r.Context(), createdBy, adminactivity.ActionGranted, adminactivity.EntityGrant, userID, grantDetails)
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

	// Resolved before revoking — SearchUsers won't return an INACTIVE grant.
	details := map[string]any{"grantId": grantID}
	var grantRole, grantModule, grantScopeType, grantScopeName string
	grantFound := false
	users, searchErr := d.Admin.SearchUsers(r.Context())
	if searchErr == nil {
		for _, u := range users {
			if u.ID != userID {
				continue
			}
			for _, g := range u.Grants {
				if g.ID == grantID {
					grantFound = true
					grantRole, grantModule = g.RoleName, g.Module
					grantScopeType, grantScopeName = g.ScopeType, g.ScopeName
					details["role"] = g.RoleName
					details["scope"] = grantScopeLabel(g.ScopeType, g.ScopeName)
				}
			}
			break
		}
	}

	// A caller can't revoke their own Admin Console access: regranting it needs
	// MANAGE_USERS, so the last holder who drops it locks everyone out.
	if caller := auth.FromContext(r.Context()); caller != nil && caller.UserID == userID {
		if searchErr != nil {
			response.WriteError(w, http.StatusServiceUnavailable,
				"Couldn't verify that grant right now. Try again in a moment.")
			return
		}
		// SHARED privileges (incl. MANAGE_USERS) are GLOBAL-only, so a SHARED grant
		// held GLOBAL is exactly the platform-admin grant — no privilege lookup needed.
		if grantFound && grantModule == "SHARED" && grantScopeType == "GLOBAL" {
			response.WriteError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
				"You can't revoke your own %s @ %s grant. Ask another platform administrator to remove it for you.",
				grantRole, grantScopeLabel(grantScopeType, grantScopeName)))
			return
		}
	}

	if label := resolveUserLabel(r.Context(), d, userID); label != "" {
		details["user"] = label
	}

	revokedBy := actor(r)
	if err := d.Grants.RevokeGrant(r.Context(), userID, grantID, revokedBy); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// entityId is the affected user (not the now-dead grant row), matching
	// handleCreateGrant's GRANTED entries — grantId lives in details instead.
	d.ActivityLog.Log(r.Context(), revokedBy, adminactivity.ActionRevoked, adminactivity.EntityGrant, userID, details)
	w.WriteHeader(http.StatusNoContent)
}

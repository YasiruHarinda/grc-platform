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

// Package handler contains the HTTP handlers for the Admin Console's Users
// screen. Kept separate from internal/user/handler
// and internal/risk/handler: every route here is gated on MANAGE_USERS GLOBAL
// specifically — never RequirePrivilegeIn, there is no scoped variant — which
// is a distinct enough authorization boundary from either module to warrant
// its own package rather than growing routes with a different gate into one
// of them.
package handler

import (
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/routeguard"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// Deps holds dependencies for the Admin Console's handlers. Composes three
// existing repositories rather than duplicating what they already do —
// admin.Repository covers only the two entity calls nothing else exposes
// (see its doc comment).
type Deps struct {
	Admin admin.Repository
	// Users provisions a platform user by uuid (Upsert with email/displayName
	// left "" — see the uuid-identity migration and Upsert's doc comment).
	Users userentity.Repository
	// Grants creates/revokes (role, scope) grants. Never nil in practice —
	// cmd/server/main.go builds it unconditionally, unlike the privilege
	// store, which is gated on TokenValidatorEnabled — but handlers still
	// guard against a nil Grants and treat it as a 500 rather than risking a
	// nil-pointer panic.
	Grants grant.Repository
	// Directory resolves/searches WSO2-org people by uuid. nil is tolerated
	// (local dev without SCIM credentials) — search then returns no results
	// and provisioning always 422s, rather than panicking.
	Directory *directory.Service
}

// RegisterRoutes mounts every Admin Console route onto mux under
// /api/v1/admin. All of them require MANAGE_USERS GLOBAL — enforced inside
// each handler, not via a wrapping middleware, to match this codebase's
// existing per-handler auth.RequirePrivilege convention (see risk/audit
// handlers).
func RegisterRoutes(mux routeguard.Router, deps Deps) {
	d := &deps

	mux.HandleFunc("GET /api/v1/admin/directory/search", d.handleSearchDirectory)
	mux.HandleFunc("GET /api/v1/admin/directory/search-external", d.handleSearchExternalDirectory)
	mux.HandleFunc("POST /api/v1/admin/users", d.handleCreateUser)
	mux.HandleFunc("GET /api/v1/admin/users", d.handleListUsers)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}/status", d.handleUpdateUserStatus)
	mux.HandleFunc("POST /api/v1/admin/users/{id}/grants", d.handleCreateGrant)
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}/grants/{grantId}", d.handleRevokeGrant)
	mux.HandleFunc("GET /api/v1/admin/roles", d.handleListRoles)
}

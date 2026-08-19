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

// Package handler contains HTTP handlers for shared user endpoints.
package handler

import (
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// Deps holds dependencies for shared user handlers.
type Deps struct {
	Users    userentity.Repository
	HREntity *hrentity.Client
	// SCIM resolves an email to the person's Asgardeo id when provisioning a
	// user, so the row carries the identity they will authenticate as.
	SCIM *scim.Client
	// Directory resolves a uuid to a name and email — the only source for
	// either, now that the platform is removing both from the database.
	// nil is tolerated: local dev with no SCIM credentials configured, in
	// which case every candidate/listing resolves to nobody rather than
	// panicking.
	Directory *directory.Service
}

// RegisterRoutes mounts shared user routes onto mux.
//
// GET /api/v1/users is Risk-module-only despite living in this shared
// package — Audit Hub has its own GET /api/v1/audit/users backed by a
// completely separate handler/service/repository stack (internal/audit/...);
// it has never called this route. Safe to shape this response around what
// Risk needs without touching Audit Hub.
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	mux.HandleFunc("GET /api/v1/me/profile", handleGetMyProfile(deps.HREntity, deps.Users))
	mux.HandleFunc("GET /api/v1/users", handleListUsers(deps.Users, deps.Directory))
	mux.HandleFunc("POST /api/v1/users/resolve", handleResolveUser(deps.Users, deps.HREntity, deps.SCIM))
}

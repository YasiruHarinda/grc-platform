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

// Package admin defines the types and data-access contract for the Admin
// Console's Users screen (see ADMIN_CONSOLE_DESIGN.md). Kept separate from
// internal/user and internal/shared/grant rather than extending either:
//
//   - internal/user.User deliberately carries no creation timestamp (nothing
//     else needed one), but the Users screen's "Date Added" column does.
//   - internal/shared/grant.Grant deliberately carries no grant id (nothing
//     else needed one — HasIn/PrivilegeMap only ever need the role/scope),
//     but revoking a specific grant does.
//
// Both gaps are Admin-Console-only needs, so the types live here rather than
// growing two packages whose existing shape is correct for their real
// callers.
package admin

import "time"

// User is one row of the Admin Console's Users list: a platform user plus
// every grant they currently hold, fetched together in a single entity round
// trip (POST /users/search with includeGrants=true) rather than N+1.
type User struct {
	ID          int    `json:"id"`
	UUID        string `json:"uuid"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	// UserType — INTERNAL or EXTERNAL. The grant editor filters its role
	// picker by this against each candidate role's AssignableUserType.
	UserType  string    `json:"userType"`
	Status    string    `json:"status"`
	CreatedOn time.Time `json:"createdOn"`
	// Grants is always a slice, never null — a freshly provisioned user
	// legitimately holds none yet.
	Grants []Grant `json:"grants"`
}

// Grant is one row of user_role_grant, resolved to names the Admin Console
// can render without a second lookup — mirrors the entity's domain.UserGrant.
type Grant struct {
	ID            int    `json:"id"`
	RoleID        int    `json:"roleId"`
	RoleName      string `json:"roleName"`
	Module        string `json:"module"` // RISK | AUDIT | SHARED
	ScopeType     string `json:"scopeType"`
	ScopeID       int    `json:"scopeId"`
	ScopeBasis    string `json:"scopeBasis,omitempty"`
	ScopeTeamType string `json:"scopeTeamType,omitempty"`
	ScopeName     string `json:"scopeName,omitempty"`
}

// Role is a row of the shared role table, for populating the grant editor's
// role picker. Every active role is returned by Repository.ListRoles — RISK,
// AUDIT, and SHARED alike.
type Role struct {
	ID          int    `json:"id"`
	RoleName    string `json:"roleName"`
	Description string `json:"description,omitempty"`
	Module      string `json:"module"`
	ScopeBasis  string `json:"scopeBasis,omitempty"`
	// AssignableUserType — INTERNAL or EXTERNAL, which kind of person this
	// role may be granted to. The grant editor filters its role picker by
	// this once a person's user type is known; the entity's CreateGrant
	// enforces it regardless of what the UI offers.
	AssignableUserType string `json:"assignableUserType"`
	Status             string `json:"status"`
}

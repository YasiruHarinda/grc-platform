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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package domain

import "time"

// Scope types for a role grant. GLOBAL means every scope the role's module
// defines — present and future — and is stored as a wildcard, never expanded
// into one row per team. A team created later is therefore covered by an
// existing GLOBAL grant immediately, with no backfill.
const (
	ScopeGlobal    = "GLOBAL"
	ScopeRiskTeam  = "RISK_TEAM"
	ScopeAuditTeam = "AUDIT_TEAM"
)

// GlobalScopeID is the sentinel stored in user_role_grant.scope_id for a GLOBAL
// grant. It is 0 rather than NULL because MySQL treats NULLs as distinct in a
// unique index: with NULL, two identical GLOBAL grants could coexist for the
// same (user, role), and revoking one would silently leave the other in force.
const GlobalScopeID = 0

// Role modules, mirroring role.module. The role decides the module; the scope
// decides the breadth. A role's own privileges cannot answer this — they may
// span both hubs — which is why module lives on the role.
const (
	ModuleRisk   = "RISK"
	ModuleAudit  = "AUDIT"
	ModuleShared = "SHARED"
)

// User types, mirroring both role.assignable_user_type and user.user_type.
// Internal and external identities live in genuinely separate Asgardeo
// organisations, so there is no third "either" value. The two are not
// symmetric, though: an INTERNAL-only role may never go to an EXTERNAL user,
// but an EXTERNAL-assignable role may be granted to either — see
// grantService.validateUserType.
const (
	UserTypeInternal = "INTERNAL"
	UserTypeExternal = "EXTERNAL"
)

// UserGrant is one row of user_role_grant, resolved to names the caller can use
// without a second lookup.
//
// Deliberately carries the role NAME rather than its privileges: callers hold
// the whole role→privilege map already (GET /role-privileges, cached), so
// sending privileges per grant would duplicate data they have and inflate a
// response that is fetched on every request.
type UserGrant struct {
	ID        int    `json:"id"`
	RoleID    int    `json:"roleId"`
	RoleName  string `json:"roleName"`
	Module    string `json:"module"`    // RISK | AUDIT | SHARED
	ScopeType string `json:"scopeType"` // GLOBAL | RISK_TEAM | AUDIT_TEAM
	ScopeID   int    `json:"scopeId"`   // 0 when scopeType is GLOBAL
	// ScopeBasis is the role's scope_basis: which column of a risk this grant
	// scopes by — SOURCE_REGISTER (where a risk was raised) or ASSIGNMENT_TEAM
	// (where the work was routed). Empty for GLOBAL-only roles.
	//
	// A risk_team row can be BOTH a register and an assignment team, so the
	// scope id alone cannot say which sense was meant. "Risk Owner @ Asgardeo"
	// means risks ASSIGNED to Asgardeo; "Risk Assigner @ Asgardeo" means risks
	// RAISED there.
	ScopeBasis string `json:"scopeBasis,omitempty"`
	// ScopeTeamType is the scoped team's own team_type (SOURCE_REGISTER,
	// ASSIGNMENT or BOTH), empty for GLOBAL. Callers need it to tell whether a
	// scope can appear on register-based pages at all: dashboards and analytics
	// are rendered per register, so a grant on an ASSIGNMENT-only team (HR,
	// Legal) contributes nothing to them.
	ScopeTeamType string `json:"scopeTeamType,omitempty"`
	// ScopeName is the team's display name, or "" for GLOBAL. Present so an
	// admin UI can render a grant without joining against the team lists.
	ScopeName string    `json:"scopeName,omitempty"`
	CreatedOn time.Time `json:"createdOn"`
	CreatedBy string    `json:"createdBy,omitempty"`
}

// UserGrantsResponse is returned by the grant read endpoints.
//
// This response is fetched on EVERY authenticated request to the GRC backend
// and is deliberately NOT cached anywhere in this service — unlike the user
// payload, which is. Revocation is a security path: an admin who removes a
// grant must see it take effect on the caller's next request, not after a cache
// TTL elapses on whichever replica happens to serve it.
type UserGrantsResponse struct {
	UserID int `json:"userId"`
	// Grants is always a slice, never null. A user with no grants is a normal,
	// expected state — an Action Owner may be any employee, holding no role at
	// all and reaching only the risks they are personally named on.
	Grants []UserGrant `json:"grants"`
}

// CreateUserGrantRequest is the payload for granting a role in a scope.
//
// ScopeID must be omitted or 0 when ScopeType is GLOBAL, and must reference an
// existing active team otherwise. The service rejects any combination that
// disagrees with the role's module before it reaches the database.
type CreateUserGrantRequest struct {
	RoleID    int    `json:"roleId"`
	ScopeType string `json:"scopeType"`
	ScopeID   int    `json:"scopeId"`
	CreatedBy string `json:"createdBy"`
}

// Role is a row of the shared role table, exposed so an admin UI can populate a
// grant editor with the roles that exist and the scopes each one accepts.
type Role struct {
	ID          int    `json:"id"`
	RoleName    string `json:"roleName"`
	Description string `json:"description,omitempty"`
	Module      string `json:"module"`
	// ScopeBasis — see UserGrant.ScopeBasis. Empty for GLOBAL-only roles.
	ScopeBasis string `json:"scopeBasis,omitempty"`
	// AssignableUserType — INTERNAL or EXTERNAL. INTERNAL means the role is
	// restricted to INTERNAL people only; EXTERNAL means it may be granted to
	// either an INTERNAL or an EXTERNAL person (see validateUserType — not a
	// third "either" value, just an asymmetric rule). Checked in CreateGrant
	// against the target user's own user_type; a UI role picker filtering by
	// this is a convenience, not the enforcement.
	AssignableUserType string `json:"assignableUserType"`
	Status             string `json:"status"`
	// GlobalOnly mirrors validateScope: true if only a GLOBAL grant of this
	// role would be accepted.
	GlobalOnly bool `json:"globalOnly"`
}

// ListRolesResponse is returned by GET /roles.
type ListRolesResponse struct {
	Roles []Role `json:"roles"`
}

// GrantCandidate is one user eligible to be picked for a role-gated field —
// e.g. Risk Owner or Management Approver — because they hold the privilege
// that field's approval action requires, in a scope that covers the field's
// context. Deliberately carries only what a picker needs: id and uuid, no
// team memberships, no timestamps — and no name or email, since the platform
// stores neither; the caller resolves one from the identity directory.
type GrantCandidate struct {
	ID int `json:"id"`
	// UUID is the candidate's Asgardeo id. Empty when the row has not been
	// backfilled yet — the caller then cannot resolve them and must drop them
	// from the picker rather than offer someone with no name.
	UUID string `json:"uuid"`
}

// GrantCandidatesResponse is returned by GET /grants/candidates.
type GrantCandidatesResponse struct {
	Candidates []GrantCandidate `json:"candidates"`
}

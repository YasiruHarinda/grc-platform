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

// Package auth exposes privilege-checking helpers built on top of the JWT
// middleware. Handlers check privileges (e.g. privilege.ComplianceApproveRisk),
// never role names.
//
// There are two kinds of check, and choosing the wrong one is the mistake this
// package is shaped to make visible:
//
//	RequirePrivilege(ctx, w, X)             — "may this user do X anywhere?"
//	RequirePrivilegeIn(ctx, w, X, teamID)   — "may this user do X on THIS thing?"
//
// A user may hold different roles in different scopes — Risk Owner in one
// register but only Risk Assigner in another; a team lead in one audit team
// but an ordinary submitter in another. The unscoped form unions those
// together, so on its own it would let someone act, as owner or lead, on a
// register or team where they hold no such authority. It exists for endpoints
// with no object in hand, and to keep existing call sites working unchanged
// during the grant migration.
//
// Rule of thumb: if a handler has a risk or team id, it must use the scoped
// form. An unscoped check in such a handler needs a comment saying why no
// scope applies.
package auth

import (
	"context"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// UserInfo re-exports the middleware type so handlers don't need to import middleware directly.
type UserInfo = middleware.UserInfo

// FromContext retrieves the authenticated user from the request context.
// Returns nil if the Auth middleware was not applied.
func FromContext(ctx context.Context) *UserInfo {
	return middleware.UserInfoFromContext(ctx)
}

// HasPrivilege returns true if the user holds the given privilege.
//
// When no privilege store was configured (TokenValidatorEnabled=false local dev),
// there is no privilege map in context and this returns true for all checks —
// mirroring how token signature verification is also skipped in that mode.
func HasPrivilege(ctx context.Context, priv string) bool {
	// If the Auth middleware wasn't applied, fail closed.
	if middleware.UserInfoFromContext(ctx) == nil {
		return false
	}
	privs := privilege.FromContext(ctx)
	if privs == nil {
		// Local dev allow-all mode (no privilege store configured).
		return true
	}
	return privs[priv]
}

// Grants returns the caller's resolved grant set, or nil when grant resolution
// was not configured (local dev). Callers that need to distinguish "sees
// everything" from "scoped to these teams" — list and dashboard scoping — read
// it directly rather than going through a privilege check.
//
// Anything reading this must also handle AllowAll: the grant set is nil in
// local dev, and treating that as "holds nothing" would scope a developer to
// nothing in the one mode that is supposed to permit everything.
func Grants(ctx context.Context) *grant.Set {
	return grant.FromContext(ctx)
}

// AllowAll reports whether the request is in local-dev allow-all mode — no
// privilege store configured (AUTH_TOKEN_VALIDATOR_ENABLED=false), so every
// privilege check passes and no grants were loaded.
//
// HasPrivilege and HasPrivilegeIn apply this themselves. It is exported for the
// row-scoping code, which reads the grant set directly and would otherwise scope
// a local developer to nothing: authenticated, waved through every route gate,
// then handed an empty dashboard and a risk list filtered to almost nothing.
//
// Returns false when the Auth middleware was not applied at all, so a request
// that never authenticated cannot reach allow-all by accident.
func AllowAll(ctx context.Context) bool {
	return middleware.UserInfoFromContext(ctx) != nil && privilege.FromContext(ctx) == nil
}

// HasPrivilegeIn returns true if the caller holds priv **in the given team's
// scope** — through a GLOBAL grant, or a grant on that team specifically.
//
// teamID must be the team whose authority governs the action. For a risk that
// is its SOURCE register, never its assignment team: assignment is an ordinary
// field any Risk Assigner can set, so letting it confer authority would make
// "route this risk to Legal" silently grant Legal's role-holders approval
// rights over it.
//
// Local dev (no privilege store configured) allows everything, mirroring
// HasPrivilege and the skipped signature verification in that mode.
func HasPrivilegeIn(ctx context.Context, priv string, teamID int) bool {
	if middleware.UserInfoFromContext(ctx) == nil {
		return false
	}
	if privilege.FromContext(ctx) == nil {
		// Local dev allow-all mode.
		return true
	}
	return grant.FromContext(ctx).HasIn(priv, teamID)
}

// RequirePrivilegeIn writes a 403 JSON response and returns false when the
// caller lacks priv in the given team's scope. Use it after the unscoped guard
// wherever a handler knows which team governs the object:
//
//	if !auth.RequirePrivilege(r.Context(), w, privilege.OwnerApproveRisk) {
//	    return
//	}
//	if !auth.RequirePrivilegeIn(r.Context(), w, privilege.OwnerApproveRisk, detail.SourceRegisterID) {
//	    return
//	}
func RequirePrivilegeIn(ctx context.Context, w http.ResponseWriter, priv string, teamID int) bool {
	if HasPrivilegeIn(ctx, priv, teamID) {
		return true
	}

	response.WriteError(
		w,
		http.StatusForbidden,
		response.ErrMsgForbidden,
	)

	return false
}

// RequirePrivilege writes a 403 JSON response and returns false when the user
// lacks the given privilege. Use it as an early-return guard in handlers:
//
//	if !auth.RequirePrivilege(r.Context(), w, privilege.ComplianceApproveRisk) {
//	    return
//	}
func RequirePrivilege(ctx context.Context, w http.ResponseWriter, priv string) bool {
	if HasPrivilege(ctx, priv) {
		return true
	}

	response.WriteError(
		w,
		http.StatusForbidden,
		response.ErrMsgForbidden,
	)

	return false
}

// RequireAnyPrivilege writes a 403 JSON response and returns false when the
// user holds none of the given privileges. Use it for dual-audience routes
// (e.g. an advisory hint visible to both submitters and reviewers):
//
//	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.SubmitEvidence, privilege.ReviewEvidence) {
//	    return
//	}
func RequireAnyPrivilege(ctx context.Context, w http.ResponseWriter, privs ...string) bool {
	for _, p := range privs {
		if HasPrivilege(ctx, p) {
			return true
		}
	}

	response.WriteError(
		w,
		http.StatusForbidden,
		response.ErrMsgForbidden,
	)

	return false
}

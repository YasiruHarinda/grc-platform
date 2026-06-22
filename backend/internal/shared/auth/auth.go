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

// Package auth exposes privilege-checking helpers built on top of the JWT middleware.
// Handlers check privileges (e.g. privilege.ApproveRisk), never role names.
// Role→privilege mappings live in the database and are loaded at startup via privilege.New.
package auth

import (
	"context"
	"net/http"

	"github.com/wso2-open-operations/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-platform/backend/internal/shared/privilege"
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

// RequirePrivilege writes a 403 JSON response and returns false when the user
// lacks the given privilege. Use it as an early-return guard in handlers:
//
//	if !auth.RequirePrivilege(r.Context(), w, privilege.ApproveRisk) {
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

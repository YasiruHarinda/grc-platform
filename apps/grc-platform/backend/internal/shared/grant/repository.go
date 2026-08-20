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

package grant

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// Repository loads a caller's grants.
type Repository interface {
	// ForUUID returns the user's internal id and their active grants, keyed on
	// the Asgardeo id the caller's token carries as `sub`.
	// A user with no platform account returns (0, nil, nil) — not an error —
	// so an authenticated caller who has never been provisioned is treated as
	// holding nothing rather than failing the request outright.
	ForUUID(ctx context.Context, uuid string) (int, []Grant, error)
	// Candidates returns every user who holds privilegeName — GLOBAL, or
	// scoped to one of teamIDs. Used to populate role-gated picker fields
	// (Risk Owner, Management Approver) with people who will actually pass
	// the same privilege check at approval time, rather than a list sourced
	// from somewhere else entirely. teamIDs may be empty.
	Candidates(ctx context.Context, privilegeName string, teamIDs []int) ([]Candidate, error)
	// CreateGrant grants a role in a scope to userID — the Admin Console's
	// grant editor. Idempotent on the entity side: re-granting something
	// already held reactivates it rather than failing.
	//
	// Deliberately uncached, same reasoning as ForUUID: a grant just created
	// must be visible to that user's very next request, not after a TTL.
	CreateGrant(ctx context.Context, userID int, req CreateGrantRequest) (Grant, error)
	// RevokeGrant deactivates (never deletes) a specific grant row — same
	// uncached reasoning as CreateGrant, in the opposite direction: a revoked
	// grant must stop working on the holder's very next request.
	RevokeGrant(ctx context.Context, userID, grantID int, revokedBy string) error
}

// CreateGrantRequest is the payload for granting a role in a scope.
// ScopeID must be omitted (0) when ScopeType is GLOBAL.
type CreateGrantRequest struct {
	RoleID    int
	ScopeType string
	ScopeID   int
	CreatedBy string
}

// Candidate is one user eligible for a role-gated picker field. Carries only
// id and uuid — the caller resolves a name/email from the identity directory
// (see risk/handler/candidates.go's resolveCandidates), which is the only
// source for either now; the entity no longer selects them.
type Candidate struct {
	ID int `json:"id"`
	// UUID is the candidate's Asgardeo id. Empty when the row has not been
	// backfilled yet — the caller then cannot resolve them and must drop them
	// from the picker rather than offer someone with no name.
	UUID string `json:"uuid"`
}

type entityRepository struct{ c *entityclient.Client }

// NewRepository returns a Compliance Entity-backed Repository.
func NewRepository(c *entityclient.Client) Repository { return &entityRepository{c: c} }

type grantsResponse struct {
	UserID int     `json:"userId"`
	Grants []Grant `json:"grants"`
}

// ForUUID fetches grants from the entity, keyed on the caller's Asgardeo id.
//
// Deliberately uncached, here and on the entity side. Grants are the revocation
// path: an admin who removes someone's role must see it take effect on that
// user's next request, not after a TTL expires on whichever replica happens to
// serve it. The role→privilege mapping is cached instead (privilege.Store),
// because it changes only on a deploy.
//
// This is why grants have their own endpoint rather than riding along on the
// user payload, which the entity caches for five minutes.
//
// The uuid comes straight from the verified token's `sub`, so no email→user
// translation sits between authenticating a caller and authorising them. During
// the identity migration a row whose uuid has not been backfilled yet resolves
// to nothing here — the same "no grants" state as an unprovisioned account,
// which fails closed rather than granting anything by accident.
func (r *entityRepository) ForUUID(ctx context.Context, uuid string) (int, []Grant, error) {
	var resp grantsResponse
	err := r.c.Get(ctx, "/grants/by-uuid/"+url.PathEscape(uuid), &resp)
	if err != nil {
		if isNotFound(err) {
			// Authenticated, but no user row here yet. Holding no grants is a
			// valid state, so this is not an error: they reach only what they
			// are personally named on, which is nothing until provisioned.
			return 0, nil, nil
		}
		// No uuid in the message: this runs on every request, gets logged via
		// slog.ErrorContext on any entity outage, and the correlation ID
		// already ties the log line to the request without putting a user
		// identifier in application logs.
		return 0, nil, fmt.Errorf("load grants: %w", err)
	}
	return resp.UserID, resp.Grants, nil
}

// Candidates fetches candidates from the entity.
//
// Deliberately uncached, same reasoning as ForUUID: a revoked grant must stop
// offering that person as a picker option on the caller's very next form load.
func (r *entityRepository) Candidates(ctx context.Context, privilegeName string, teamIDs []int) ([]Candidate, error) {
	q := url.Values{}
	q.Set("privilege", privilegeName)
	for _, id := range teamIDs {
		q.Add("teamId", strconv.Itoa(id))
	}
	var resp struct {
		Candidates []Candidate `json:"candidates"`
	}
	if err := r.c.Get(ctx, "/grants/candidates?"+q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("load candidates: %w", err)
	}
	return resp.Candidates, nil
}

// CreateGrant creates (or reactivates) a grant via the entity's
// POST /grants/user/{id}. See CreateGrantRequest for the payload shape.
func (r *entityRepository) CreateGrant(ctx context.Context, userID int, req CreateGrantRequest) (Grant, error) {
	body := map[string]any{
		"roleId":    req.RoleID,
		"scopeType": req.ScopeType,
		"scopeId":   req.ScopeID,
		"createdBy": req.CreatedBy,
	}
	var g Grant
	if err := r.c.Post(ctx, fmt.Sprintf("/grants/user/%d", userID), body, &g); err != nil {
		return Grant{}, fmt.Errorf("create grant: %w", err)
	}
	return g, nil
}

// RevokeGrant deactivates a grant via the entity's
// DELETE /grants/user/{id}/{grantId}?revokedBy=.
func (r *entityRepository) RevokeGrant(ctx context.Context, userID, grantID int, revokedBy string) error {
	q := url.Values{}
	q.Set("revokedBy", revokedBy)
	path := fmt.Sprintf("/grants/user/%d/%d?%s", userID, grantID, q.Encode())
	if err := r.c.Delete(ctx, path); err != nil {
		return fmt.Errorf("revoke grant: %w", err)
	}
	return nil
}

// isNotFound reports whether err is the entity's 404, mirroring
// internal/user/entity's notFound helper.
func isNotFound(err error) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

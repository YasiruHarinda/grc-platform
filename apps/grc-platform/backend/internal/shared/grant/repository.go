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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// Repository loads a caller's grants.
type Repository interface {
	// ForEmail returns the user's internal id and their active grants.
	// A user with no platform account returns (0, nil, nil) — not an error —
	// so an authenticated caller who has never been provisioned is treated as
	// holding nothing rather than failing the request outright.
	ForEmail(ctx context.Context, email string) (int, []Grant, error)
}

type entityRepository struct{ c *entityclient.Client }

// NewRepository returns a Compliance Entity-backed Repository.
func NewRepository(c *entityclient.Client) Repository { return &entityRepository{c: c} }

type grantsResponse struct {
	UserID int     `json:"userId"`
	Grants []Grant `json:"grants"`
}

// ForEmail fetches grants from the entity.
//
// Deliberately uncached, here and on the entity side. Grants are the revocation
// path: an admin who removes someone's role must see it take effect on that
// user's next request, not after a TTL expires on whichever replica happens to
// serve it. The role→privilege mapping is cached instead (privilege.Store),
// because it changes only on a deploy.
//
// This is why grants have their own endpoint rather than riding along on the
// user payload, which the entity caches for five minutes.
func (r *entityRepository) ForEmail(ctx context.Context, email string) (int, []Grant, error) {
	var resp grantsResponse
	err := r.c.Get(ctx, "/grants/by-email/"+url.PathEscape(email), &resp)
	if err != nil {
		if isNotFound(err) {
			// Authenticated, but no user row here yet. Holding no grants is a
			// valid state, so this is not an error: they reach only what they
			// are personally named on, which is nothing until provisioned.
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("load grants for %s: %w", email, err)
	}
	return resp.UserID, resp.Grants, nil
}

// isNotFound reports whether err is the entity's 404, mirroring
// internal/user/entity's notFound helper.
func isNotFound(err error) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

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

// Package entity provides an HTTP-client implementation of user.Repository
// backed by the Compliance Entity instead of direct MySQL access. The `user`
// table is owned by the entity; the GRC backend never queries it directly.
package entity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// pageLimit matches the entity's max page size; List pages through all results.
const pageLimit = 100

// statusActive is the only user status the shared dropdown lists should show.
const statusActive = "ACTIVE"

type repository struct{ c *entityclient.Client }

// NewRepository returns a Compliance Entity-backed user.Repository.
func NewRepository(c *entityclient.Client) user.Repository {
	return &repository{c: c}
}

// entUser mirrors the entity's User JSON (camelCase, createdOn/updatedOn),
// which differs from the backend's user.User (snake_case, no timestamps).
// AuditTeamIDs is deliberately not mirrored here — it's a many-to-many slice
// on the entity side and nothing in the GRC backend or webapp consumes it.
// The entity itself carries no Email/DisplayName (the `user` table stores
// neither — see shared.sql); user.User's own Email/DisplayName fields are
// populated by callers from the identity directory instead (see
// user/handler/users.go, resolve.go), never from this response.
type entUser struct {
	ID          int    `json:"id"`
	UUID        string `json:"uuid"`
	UserType    string `json:"userType"`
	RiskTeamIDs []int  `json:"riskTeamIds"`
	Status      string `json:"status"`
}

func (u entUser) toModel() *user.User {
	return &user.User{
		ID:          u.ID,
		UUID:        u.UUID,
		Status:      u.Status,
		RiskTeamIDs: u.RiskTeamIDs,
	}
}

func (r *repository) GetByID(ctx context.Context, id int) (*user.User, error) {
	var u entUser
	if err := r.c.Get(ctx, fmt.Sprintf("/users/%d", id), &u); err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u.toModel(), nil
}

func (r *repository) GetByUUID(ctx context.Context, uuid string) (*user.User, error) {
	var u entUser
	if err := r.c.Get(ctx, "/users/by-uuid/"+url.PathEscape(uuid), &u); err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by uuid: %w", err)
	}
	return u.toModel(), nil
}

// Upsert provisions an account for an employee picked from an HR entity search
// (e.g. as a risk's Action Owner) who may never have signed in to grc-platform.
// POST /users is an upsert on the entity side, keyed on uuid — it inserts when
// the uuid is new and just refreshes updated_by when it isn't — so this is a
// single round trip with no read-then-write race. userType/status are left
// empty so the entity applies its own defaults (INTERNAL / ACTIVE).
//
// uuid is required — see the Repository interface doc for why: the caller
// must have already resolved one (e.g. against the identity directory)
// before calling this.
func (r *repository) Upsert(ctx context.Context, uuid, actor string) (*user.User, error) {
	body := map[string]any{
		"uuid":      uuid,
		"createdBy": actor,
	}
	var u entUser
	if err := r.c.Post(ctx, "/users", body, &u); err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	return u.toModel(), nil
}

// UpsertTyped is Upsert plus an explicit userType — see the Repository
// interface doc. An empty userType behaves exactly like Upsert (the entity
// defaults it to INTERNAL), so this subsumes Upsert's contract rather than
// diverging from it.
func (r *repository) UpsertTyped(ctx context.Context, uuid, userType, actor string) (*user.User, error) {
	body := map[string]any{
		"uuid":      uuid,
		"userType":  userType,
		"createdBy": actor,
	}
	var u entUser
	if err := r.c.Post(ctx, "/users", body, &u); err != nil {
		return nil, fmt.Errorf("upsert user (typed): %w", err)
	}
	return u.toModel(), nil
}

// UpdateStatus sets a user's status via the entity's existing PATCH
// /users/{id} — AuditTeamIDs is left nil (unspecified), which the entity
// treats as "leave audit team membership alone" (see UpdateUserRequest's doc
// comment on the entity side), so this touches status only.
func (r *repository) UpdateStatus(ctx context.Context, id int, status, actor string) (*user.User, error) {
	body := map[string]any{
		"status":    status,
		"updatedBy": actor,
	}
	var u entUser
	if err := r.c.Patch(ctx, fmt.Sprintf("/users/%d", id), body, &u); err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("update user status: %w", err)
	}
	return u.toModel(), nil
}

// List returns every active user, paging through the entity's search endpoint.
func (r *repository) List(ctx context.Context) ([]*user.User, error) {
	var all []*user.User
	for offset := 0; ; offset += pageLimit {
		body := map[string]any{
			"statusKey":  statusActive,
			"pagination": map[string]int{"limit": pageLimit, "offset": offset},
		}
		var resp struct {
			Users []entUser `json:"users"`
		}
		if err := r.c.Post(ctx, "/users/search", body, &resp); err != nil {
			return nil, fmt.Errorf("list users: %w", err)
		}
		for _, u := range resp.Users {
			all = append(all, u.toModel())
		}
		if len(resp.Users) < pageLimit {
			return all, nil
		}
	}
}

// notFound reports whether err is the entity's 404, so callers can map a
// missing user to (nil, nil) instead of surfacing a transport error.
func notFound(err error) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

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
type entUser struct {
	ID          int    `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	UserType    string `json:"userType"`
	AuditTeamID *int   `json:"auditTeamId"`
	RiskTeamID  *int   `json:"riskTeamId"`
	Status      string `json:"status"`
}

func (u entUser) toModel() *user.User {
	return &user.User{
		ID:          u.ID,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Status:      u.Status,
		AuditTeamID: u.AuditTeamID,
		RiskTeamID:  u.RiskTeamID,
	}
}

func (r *repository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	var u entUser
	if err := r.c.Get(ctx, "/users/by-email/"+url.PathEscape(email), &u); err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u.toModel(), nil
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

// Upsert provisions an account for an employee picked from an HR entity search
// (e.g. as a risk's Action Owner) who may never have signed in to grc-platform.
// POST /users is an upsert on the entity side — it inserts when the email is
// new and refreshes display_name when it isn't — so this is a single round trip
// with no read-then-write race. userType/status are left empty so the entity
// applies its own defaults (INTERNAL / ACTIVE).
func (r *repository) Upsert(ctx context.Context, email, displayName, actorEmail string) (*user.User, error) {
	body := map[string]any{
		"email":       email,
		"displayName": displayName,
		"createdBy":   actorEmail,
	}
	var u entUser
	if err := r.c.Post(ctx, "/users", body, &u); err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
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

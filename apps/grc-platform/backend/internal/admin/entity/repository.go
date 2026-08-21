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

// Package entity provides an HTTP-client implementation of admin.Repository
// backed by the Compliance Entity.
package entity

import (
	"context"
	"fmt"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// pageLimit is the entity's maximum page size; SearchUsers pages through all
// results.
const pageLimit = 100

type repository struct{ c *entityclient.Client }

// NewRepository returns a Compliance Entity-backed admin.Repository.
func NewRepository(c *entityclient.Client) admin.Repository { return &repository{c: c} }

// entGrant mirrors the entity's domain.UserGrant JSON.
type entGrant struct {
	ID            int    `json:"id"`
	RoleID        int    `json:"roleId"`
	RoleName      string `json:"roleName"`
	Module        string `json:"module"`
	ScopeType     string `json:"scopeType"`
	ScopeID       int    `json:"scopeId"`
	ScopeBasis    string `json:"scopeBasis,omitempty"`
	ScopeTeamType string `json:"scopeTeamType,omitempty"`
	ScopeName     string `json:"scopeName,omitempty"`
}

// SearchUsers calls the entity's POST /users/search with includeGrants=true,
// so the Admin Console's user list gets every row's grants in one round trip.
// Every status is included deliberately — an Admin managing role grants needs
// to see (and reactivate) an INACTIVE user too, unlike the Risk module's
// dropdowns, which only ever want ACTIVE ones.
//
// query is unused: the entity's SearchUsersRequest dropped free-text search
// (no email/display name left to search once the uuid-identity migration
// removed them from the user table — see its own doc comment). The one
// caller of this method never passes a non-empty query either; filtering by
// name/email happens client-side in UsersPage against directory-resolved
// names instead.
func (r *repository) SearchUsers(ctx context.Context, query string) ([]admin.User, error) {
	type entUser struct {
		ID          int        `json:"id"`
		UUID        string     `json:"uuid"`
		Email       string     `json:"email"`
		DisplayName string     `json:"displayName"`
		UserType    string     `json:"userType"`
		Status      string     `json:"status"`
		CreatedOn   time.Time  `json:"createdOn"`
		Grants      []entGrant `json:"grants"`
	}

	var users []entUser
	for offset := 0; ; offset += pageLimit {
		body := map[string]any{
			"includeGrants": true,
			"pagination":    map[string]int{"limit": pageLimit, "offset": offset},
		}
		var resp struct {
			Users []entUser `json:"users"`
		}
		if err := r.c.Post(ctx, "/users/search", body, &resp); err != nil {
			return nil, fmt.Errorf("search users: %w", err)
		}
		users = append(users, resp.Users...)
		if len(resp.Users) < pageLimit {
			break
		}
	}

	out := make([]admin.User, 0, len(users))
	for _, u := range users {
		grants := make([]admin.Grant, 0, len(u.Grants))
		for _, g := range u.Grants {
			grants = append(grants, admin.Grant{
				ID: g.ID, RoleID: g.RoleID, RoleName: g.RoleName, Module: g.Module,
				ScopeType: g.ScopeType, ScopeID: g.ScopeID, ScopeBasis: g.ScopeBasis,
				ScopeTeamType: g.ScopeTeamType, ScopeName: g.ScopeName,
			})
		}
		out = append(out, admin.User{
			ID: u.ID, UUID: u.UUID, DisplayName: u.DisplayName, Email: u.Email,
			UserType: u.UserType, Status: u.Status, CreatedOn: u.CreatedOn, Grants: grants,
		})
	}
	return out, nil
}

// ListRoles calls the entity's GET /roles — every active role, RISK, AUDIT,
// and SHARED alike. See admin.Repository.ListRoles for why AUDIT roles are no
// longer withheld.
func (r *repository) ListRoles(ctx context.Context) ([]admin.Role, error) {
	var resp struct {
		Roles []admin.Role `json:"roles"`
	}
	if err := r.c.Get(ctx, "/roles", &resp); err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	return resp.Roles, nil
}

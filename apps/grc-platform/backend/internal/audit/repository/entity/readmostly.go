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

// Package entity provides HTTP-client implementations of the audit repository
// interfaces, backed by the Compliance Entity instead of direct MySQL access.
package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

const pageLimit = 100 // entity maxLimit; List methods page through all results.

// pageBody is used only by the unscoped List() methods (auditRepo, controlRepo)
// that back internal existence/uniqueness checks (checkNameAvailable,
// sanitizedControlNumbers) and must see every row regardless of caller
// identity. The entity service no longer treats an omitted "scope" as
// unrestricted — it denies by default — so ScopeAll must be sent explicitly.
func pageBody(offset int) map[string]any {
	return map[string]any{
		"scope":      model.ScopeAll,
		"pagination": map[string]int{"limit": pageLimit, "offset": offset},
	}
}

// statusActive is the only status these list endpoints should show — mirrors
// internal/user/entity/repository.go's statusActive. Every search endpoint
// here treats an omitted statusKey as "all statuses", so callers that want
// only ACTIVE rows (dropdowns, catalogs) must send it explicitly.
const statusActive = "ACTIVE"

func activeStatusPageBody(offset int) map[string]any {
	return map[string]any{
		"statusKey":  statusActive,
		"pagination": map[string]int{"limit": pageLimit, "offset": offset},
	}
}

// ── Frameworks ────────────────────────────────────────────────────────────────

type frameworkRepo struct{ c *entityclient.Client }

// NewFrameworkRepository returns an entity-backed FrameworkRepository.
func NewFrameworkRepository(c *entityclient.Client) repository.FrameworkRepository {
	return &frameworkRepo{c: c}
}

func (r *frameworkRepo) List(ctx context.Context, scope model.Scope, userID int, scopeTeamIDs []int) ([]*model.AuditFramework, error) {
	var all []*model.AuditFramework
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Frameworks []*model.AuditFramework `json:"frameworks"`
		}
		body := map[string]any{
			"statusKey":    statusActive,
			"scope":        scope,
			"userId":       userID,
			"scopeTeamIds": scopeTeamIDs,
			"pagination":   map[string]int{"limit": pageLimit, "offset": offset},
		}
		if err := r.c.Post(ctx, "/audit/frameworks/search", body, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Frameworks...)
		if len(resp.Frameworks) < pageLimit {
			return all, nil
		}
	}
}

func (r *frameworkRepo) GetByID(ctx context.Context, id int) (*model.AuditFramework, error) {
	var fw model.AuditFramework
	if err := r.c.Get(ctx, fmt.Sprintf("/audit/frameworks/%d", id), &fw); err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &fw, nil
}

func (r *frameworkRepo) Create(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error) {
	body := map[string]any{"name": req.Name, "status": "ACTIVE", "createdBy": createdBy}
	var fw model.AuditFramework
	if err := r.c.Post(ctx, "/audit/frameworks", body, &fw); err != nil {
		return nil, err
	}
	return &fw, nil
}

// ── Products ──────────────────────────────────────────────────────────────────

type productRepo struct{ c *entityclient.Client }

// NewProductRepository returns an entity-backed ProductRepository.
func NewProductRepository(c *entityclient.Client) repository.ProductRepository {
	return &productRepo{c: c}
}

func (r *productRepo) List(ctx context.Context) ([]*model.AuditProduct, error) {
	var all []*model.AuditProduct
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Products []*model.AuditProduct `json:"products"`
		}
		if err := r.c.Post(ctx, "/audit/products/search", activeStatusPageBody(offset), &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Products...)
		if len(resp.Products) < pageLimit {
			return all, nil
		}
	}
}

func (r *productRepo) GetByID(ctx context.Context, id int) (*model.AuditProduct, error) {
	var p model.AuditProduct
	if err := r.c.Get(ctx, fmt.Sprintf("/audit/products/%d", id), &p); err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *productRepo) Create(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error) {
	body := map[string]any{"name": req.Name, "status": "ACTIVE", "createdBy": createdBy}
	var p model.AuditProduct
	if err := r.c.Post(ctx, "/audit/products", body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ── Users ─────────────────────────────────────────────────────────────────────

type userRepo struct{ c *entityclient.Client }

// NewUserRepository returns an entity-backed UserRepository.
func NewUserRepository(c *entityclient.Client) repository.UserRepository {
	return &userRepo{c: c}
}

func (r *userRepo) List(ctx context.Context) ([]*model.UserRef, error) {
	var all []*model.UserRef
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Users []*model.UserRef `json:"users"`
		}
		if err := r.c.Post(ctx, "/users/search", activeStatusPageBody(offset), &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Users...)
		if len(resp.Users) < pageLimit {
			return all, nil
		}
	}
}

// GetByID is deliberately unfiltered by status (unlike List, which only shows
// ACTIVE users for dropdowns) — the notification dispatcher needs to see an
// INACTIVE/REMOVED user's real status in order to skip emailing them.
func (r *userRepo) GetByID(ctx context.Context, id int) (*model.UserRef, error) {
	var u model.UserRef
	if err := r.c.Get(ctx, fmt.Sprintf("/users/%d", id), &u); err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// ── Teams ─────────────────────────────────────────────────────────────────────

type teamRepo struct{ c *entityclient.Client }

// NewTeamRepository returns an entity-backed TeamRepository.
func NewTeamRepository(c *entityclient.Client) repository.TeamRepository {
	return &teamRepo{c: c}
}

func (r *teamRepo) List(ctx context.Context) ([]*model.AuditTeam, error) {
	var all []*model.AuditTeam
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Teams []*model.AuditTeam `json:"teams"`
		}
		if err := r.c.Post(ctx, "/audit/teams/search", activeStatusPageBody(offset), &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Teams...)
		if len(resp.Teams) < pageLimit {
			return all, nil
		}
	}
}

// ListAll is List without the ACTIVE-only filter — an empty statusKey means
// every status (see the entity's SearchAuditTeamsRequest doc comment) — and
// with Status carried through on each row, which List's dropdown callers have
// no use for and so leave unpopulated.
func (r *teamRepo) ListAll(ctx context.Context) ([]*model.AuditTeam, error) {
	var all []*model.AuditTeam
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Teams []*model.AuditTeam `json:"teams"`
		}
		body := map[string]any{"pagination": map[string]int{"limit": pageLimit, "offset": offset}}
		if err := r.c.Post(ctx, "/audit/teams/search", body, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Teams...)
		if len(resp.Teams) < pageLimit {
			return all, nil
		}
	}
}

func (r *teamRepo) Create(ctx context.Context, req model.CreateTeamRequest, createdBy string) (*model.AuditTeam, error) {
	body := map[string]any{"name": req.Name, "status": req.Status, "createdBy": createdBy}
	var t model.AuditTeam
	if err := r.c.Post(ctx, "/audit/teams", body, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *teamRepo) Update(ctx context.Context, id int, req model.UpdateTeamRequest, updatedBy string) (*model.AuditTeam, error) {
	body := map[string]any{"name": req.Name, "status": req.Status, "updatedBy": updatedBy}
	var t model.AuditTeam
	if err := r.c.Patch(ctx, fmt.Sprintf("/audit/teams/%d", id), body, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ── Framework Controls ────────────────────────────────────────────────────────

type frameworkControlRepo struct{ c *entityclient.Client }

// NewFrameworkControlRepository returns an entity-backed FrameworkControlRepository.
func NewFrameworkControlRepository(c *entityclient.Client) repository.FrameworkControlRepository {
	return &frameworkControlRepo{c: c}
}

func (r *frameworkControlRepo) ListCurrent(ctx context.Context, frameworkID int) ([]*model.AuditFrameworkControl, error) {
	var resp struct {
		Controls []*model.AuditFrameworkControl `json:"controls"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/audit/frameworks/%d/controls", frameworkID), &resp); err != nil {
		return nil, err
	}
	return resp.Controls, nil
}

func (r *frameworkControlRepo) Create(ctx context.Context, frameworkID int, req model.CreateFrameworkControlRequest, createdBy string) (*model.AuditFrameworkControl, error) {
	body := map[string]any{
		"controlNumber":       req.ControlNumber,
		"description":         req.Description,
		"evidenceRequirement": req.EvidenceRequirement,
		"requirementType":     req.RequirementType,
		"controlType":         req.ControlType,
		"scope":               req.Scope,
		"createdBy":           createdBy,
	}
	var fc model.AuditFrameworkControl
	if err := r.c.Post(ctx, fmt.Sprintf("/audit/frameworks/%d/controls", frameworkID), body, &fc); err != nil {
		return nil, err
	}
	return &fc, nil
}

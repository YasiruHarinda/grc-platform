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

package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// withField marshals a request struct to a JSON map and adds/overrides extra
// fields (e.g. createdBy/updatedBy, which the entity expects in the body but the
// backend passes as a separate argument). Field names already match (camelCase).
func withField(body any, extra map[string]any) map[string]any {
	m := map[string]any{}
	if b, err := json.Marshal(body); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// entAudit mirrors the entity's Audit JSON, which is flatter than the backend
// model (flat framework/product fields, control counts, createdOn/updatedOn).
type entAudit struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	FrameworkID      int       `json:"frameworkId"`
	FrameworkName    string    `json:"frameworkName"`
	ProductID        int       `json:"productId"`
	ProductName      string    `json:"productName"`
	PeriodStart      string    `json:"periodStart"`
	PeriodEnd        string    `json:"periodEnd"`
	Status           string    `json:"status"`
	ScopeDescription *string   `json:"scopeDescription"`
	ControlsTotal    int       `json:"controlsTotal"`
	ControlsApproved int       `json:"controlsApproved"`
	ControlsOverdue  int       `json:"controlsOverdue"`
	CreatedOn        time.Time `json:"createdOn"`
	UpdatedOn        time.Time `json:"updatedOn"`
}

func (a entAudit) toModel() *model.Audit {
	return &model.Audit{
		ID:               a.ID,
		Name:             a.Name,
		Framework:        model.AuditFrameworkRef{ID: a.FrameworkID, Name: a.FrameworkName},
		Product:          model.AuditProductRef{ID: a.ProductID, Name: a.ProductName},
		PeriodStart:      a.PeriodStart,
		PeriodEnd:        a.PeriodEnd,
		Status:           a.Status,
		ScopeDescription: a.ScopeDescription,
		ControlCounts: model.ControlCounts{
			Total:    a.ControlsTotal,
			Approved: a.ControlsApproved,
			Overdue:  a.ControlsOverdue,
		},
		CreatedAt: a.CreatedOn,
		UpdatedAt: a.UpdatedOn,
	}
}

type auditRepo struct{ c *entityclient.Client }

// NewAuditRepository returns an entity-backed AuditRepository.
func NewAuditRepository(c *entityclient.Client) repository.AuditRepository {
	return &auditRepo{c: c}
}

func (r *auditRepo) List(ctx context.Context) ([]*model.Audit, error) {
	var all []*model.Audit
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Audits []entAudit `json:"audits"`
		}
		if err := r.c.Post(ctx, "/audits/search", pageBody(offset), &resp); err != nil {
			return nil, err
		}
		for _, a := range resp.Audits {
			all = append(all, a.toModel())
		}
		if len(resp.Audits) < pageLimit {
			return all, nil
		}
	}
}

func (r *auditRepo) ListScoped(ctx context.Context, scope model.Scope, userID int, scopeTeamIDs []int) ([]*model.Audit, error) {
	var all []*model.Audit
	for offset := 0; ; offset += pageLimit {
		var resp struct {
			Audits []entAudit `json:"audits"`
		}
		body := map[string]any{
			"scope":        scope,
			"userId":       userID,
			"scopeTeamIds": scopeTeamIDs,
			"pagination":   map[string]int{"limit": pageLimit, "offset": offset},
		}
		if err := r.c.Post(ctx, "/audits/search", body, &resp); err != nil {
			return nil, err
		}
		for _, a := range resp.Audits {
			all = append(all, a.toModel())
		}
		if len(resp.Audits) < pageLimit {
			return all, nil
		}
	}
}

// InScope reports whether id is visible to userID at scope, by reusing
// /audits/search (the same scoped query ListScoped uses) filtered to just this
// one audit id — avoids a bespoke endpoint for what is otherwise the same check.
func (r *auditRepo) InScope(ctx context.Context, id int, scope model.Scope, userID int, scopeTeamIDs []int) (bool, error) {
	var resp struct {
		Audits []entAudit `json:"audits"`
	}
	body := map[string]any{
		"auditIds":     []int{id},
		"scope":        scope,
		"userId":       userID,
		"scopeTeamIds": scopeTeamIDs,
		"pagination":   map[string]int{"limit": 1, "offset": 0},
	}
	if err := r.c.Post(ctx, "/audits/search", body, &resp); err != nil {
		return false, err
	}
	return len(resp.Audits) > 0, nil
}

func (r *auditRepo) GetByID(ctx context.Context, id int) (*model.Audit, error) {
	var a entAudit
	if err := r.c.Get(ctx, fmt.Sprintf("/audits/%d", id), &a); err != nil {
		return nil, err
	}
	return a.toModel(), nil
}

func (r *auditRepo) Create(ctx context.Context, req model.CreateAuditRequest, createdBy string) (*model.Audit, error) {
	var a entAudit
	body := withField(req, map[string]any{"createdBy": createdBy})
	if err := r.c.Post(ctx, "/audits", body, &a); err != nil {
		return nil, err
	}
	return a.toModel(), nil
}

func (r *auditRepo) Update(ctx context.Context, id int, req model.UpdateAuditRequest, updatedBy string) error {
	body := withField(req, map[string]any{"updatedBy": updatedBy})
	return r.c.Patch(ctx, fmt.Sprintf("/audits/%d", id), body, nil)
}

func (r *auditRepo) Delete(ctx context.Context, id int, deletedBy string) error {
	// The entity requires the acting user for the soft-delete audit trail.
	return r.c.Delete(ctx, fmt.Sprintf("/audits/%d?deletedBy=%s", id, url.QueryEscape(deletedBy)))
}

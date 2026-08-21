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
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type riskCategoryRepository struct{ c *entityclient.Client }

// NewRiskCategoryRepository creates a Compliance Entity-backed
// repository.RiskCategoryRepository.
func NewRiskCategoryRepository(c *entityclient.Client) repository.RiskCategoryRepository {
	return &riskCategoryRepository{c: c}
}

type entRiskCategory struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// listRiskCategoriesResponse mirrors the entity's GET /risk/categories
// wrapper object — a flat list, no pagination, since the table only ever
// holds a small fixed-but-extensible set of rows.
type listRiskCategoriesResponse struct {
	Categories []entRiskCategory `json:"categories"`
}

// List returns every risk category ordered by name, matching the entity's query.
func (r *riskCategoryRepository) List(ctx context.Context) ([]*model.RiskCategory, error) {
	var resp listRiskCategoriesResponse
	if err := r.c.Get(ctx, "/risk/categories", &resp); err != nil {
		return nil, fmt.Errorf("list risk categories: %w", err)
	}
	cats := make([]*model.RiskCategory, 0, len(resp.Categories))
	for _, c := range resp.Categories {
		cats = append(cats, &model.RiskCategory{ID: c.ID, Name: c.Name, Description: c.Description})
	}
	return cats, nil
}

// Create adds a new risk category via the entity's POST /risk/categories.
// name uniqueness (uq_risk_category_name) is enforced by the entity, which
// maps a duplicate into a 409 — surfaced here as an *apierror.Error the
// caller's response.MapServiceError already knows how to translate.
func (r *riskCategoryRepository) Create(ctx context.Context, req model.CreateRiskCategoryRequest, createdBy string) (*model.RiskCategory, error) {
	body := map[string]any{
		"name":        req.Name,
		"description": req.Description,
		"createdBy":   createdBy,
	}
	var c entRiskCategory
	if err := r.c.Post(ctx, "/risk/categories", body, &c); err != nil {
		return nil, fmt.Errorf("create risk category: %w", err)
	}
	return &model.RiskCategory{ID: c.ID, Name: c.Name, Description: c.Description}, nil
}

// Update edits a risk category via the entity's PATCH /risk/categories/{id}.
func (r *riskCategoryRepository) Update(ctx context.Context, id int, req model.UpdateRiskCategoryRequest, updatedBy string) (*model.RiskCategory, error) {
	body := map[string]any{
		"name":        req.Name,
		"description": req.Description,
		"updatedBy":   updatedBy,
	}
	var c entRiskCategory
	if err := r.c.Patch(ctx, fmt.Sprintf("/risk/categories/%d", id), body, &c); err != nil {
		return nil, fmt.Errorf("update risk category %d: %w", id, err)
	}
	return &model.RiskCategory{ID: c.ID, Name: c.Name, Description: c.Description}, nil
}

// Delete removes a risk category via the entity's DELETE /risk/categories/{id}.
// The entity itself refuses (409) when the category is still used by a risk —
// see its DeleteRiskCategory doc comment.
func (r *riskCategoryRepository) Delete(ctx context.Context, id int) error {
	if err := r.c.Delete(ctx, fmt.Sprintf("/risk/categories/%d", id)); err != nil {
		return fmt.Errorf("delete risk category %d: %w", id, err)
	}
	return nil
}

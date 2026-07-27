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

type complianceReferenceRepository struct{ c *entityclient.Client }

// NewComplianceReferenceRepository creates a Compliance Entity-backed
// repository.ComplianceReferenceRepository.
func NewComplianceReferenceRepository(c *entityclient.Client) repository.ComplianceReferenceRepository {
	return &complianceReferenceRepository{c: c}
}

// entComplianceReference is the entity's camelCase representation of a
// compliance reference. The entity also returns createdOn/updatedOn, which
// model.ComplianceReference does not carry and which are dropped here.
type entComplianceReference struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// searchReferencesResponse mirrors the entity's wrapper object; the search
// endpoint returns {"references": [...], "total": n, ...} rather than an array.
type searchReferencesResponse struct {
	References []entComplianceReference `json:"references"`
	Total      int                      `json:"total"`
}

// List returns every compliance reference ordered by name, matching the MySQL
// query. The entity applies the same ORDER BY name, so ordering is preserved.
// Neither side filters by status — the table has no status column.
//
// The entity requires pagination where the MySQL query returned every row, so
// this pages until a short page. The result is left nil when empty, matching
// the MySQL implementation; the handler normalises nil to [] for JSON.
func (r *complianceReferenceRepository) List(ctx context.Context) ([]*model.ComplianceReference, error) {
	var refs []*model.ComplianceReference
	for offset := 0; ; offset += pageLimit {
		body := map[string]any{
			"searchQuery": "",
			"pagination":  map[string]int{"limit": pageLimit, "offset": offset},
		}
		var resp searchReferencesResponse
		if err := r.c.Post(ctx, "/risk/compliance-references/search", body, &resp); err != nil {
			return nil, fmt.Errorf("list compliance references: %w", err)
		}
		for _, ref := range resp.References {
			refs = append(refs, &model.ComplianceReference{
				ID:          ref.ID,
				Name:        ref.Name,
				Description: ref.Description,
			})
		}
		if len(resp.References) < pageLimit {
			return refs, nil
		}
	}
}

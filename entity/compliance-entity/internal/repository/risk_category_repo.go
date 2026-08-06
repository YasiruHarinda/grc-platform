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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskCategoryRepository defines persistence operations for the risk_category table.
type RiskCategoryRepository interface {
	ListRiskCategories(ctx context.Context) ([]domain.RiskCategory, error)
}

type riskCategoryRepo struct{ db *sql.DB }

// NewRiskCategoryRepository constructs a RiskCategoryRepository.
func NewRiskCategoryRepository(db *sql.DB) RiskCategoryRepository { return &riskCategoryRepo{db: db} }

// ListRiskCategories returns every seeded risk category ordered by name.
func (r *riskCategoryRepo) ListRiskCategories(ctx context.Context) ([]domain.RiskCategory, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, description FROM risk_category ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("risk_category.List: %w", err)
	}
	defer rows.Close()

	var cats []domain.RiskCategory
	for rows.Next() {
		var c domain.RiskCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Description); err != nil {
			return nil, fmt.Errorf("risk_category.List scan: %w", err)
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

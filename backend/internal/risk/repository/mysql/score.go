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

package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

type riskScoreRepository struct{ db *sql.DB }

// NewRiskScoreRepository creates a MySQL-backed repository.RiskScoreRepository.
func NewRiskScoreRepository(db *sql.DB) repository.RiskScoreRepository {
	return &riskScoreRepository{db: db}
}

func (r *riskScoreRepository) List(ctx context.Context) ([]*model.RiskScore, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, likelihood, impact, risk_rating, risk_level, color_code
		FROM risk_score
		ORDER BY likelihood, impact`)
	if err != nil {
		return nil, fmt.Errorf("list risk scores: %w", err)
	}
	defer rows.Close()

	var scores []*model.RiskScore
	for rows.Next() {
		s := &model.RiskScore{}
		if err := rows.Scan(&s.ID, &s.Likelihood, &s.Impact, &s.RiskRating, &s.RiskLevel, &s.ColorCode); err != nil {
			return nil, fmt.Errorf("scan risk score row: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

func (r *riskScoreRepository) Create(ctx context.Context, req model.CreateRiskScoreRequest, createdBy string) (*model.RiskScore, error) {
	// TODO: implement risk_score INSERT
	return nil, nil
}

func (r *riskScoreRepository) Update(ctx context.Context, id int, req model.UpdateRiskScoreRequest, updatedBy string) error {
	// TODO: implement risk_score UPDATE
	return nil
}

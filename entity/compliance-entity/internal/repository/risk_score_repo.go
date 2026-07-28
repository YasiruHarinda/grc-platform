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

// RiskScoreRepository defines persistence operations for the risk_score table.
type RiskScoreRepository interface {
	ListRiskScores(ctx context.Context) ([]domain.RiskScore, error)
}

type riskScoreRepo struct{ db *sql.DB }

// NewRiskScoreRepository constructs a RiskScoreRepository.
func NewRiskScoreRepository(db *sql.DB) RiskScoreRepository { return &riskScoreRepo{db: db} }

// ListRiskScores returns the full score matrix ordered by likelihood then
// impact. Ordering by risk_rating instead would tie — (1,2) and (2,1) both rate
// 2, as do (1,3) and (3,1) — and MySQL does not guarantee the order of tied
// rows, so the sequence could vary between executions. Likelihood/impact is
// total, and matches the order the GRC backend has always returned.
func (r *riskScoreRepo) ListRiskScores(ctx context.Context) ([]domain.RiskScore, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, likelihood, impact, risk_rating, risk_level, color_code FROM risk_score ORDER BY likelihood, impact")
	if err != nil {
		return nil, fmt.Errorf("risk_score.List: %w", err)
	}
	defer rows.Close()

	var scores []domain.RiskScore
	for rows.Next() {
		var s domain.RiskScore
		if err := rows.Scan(&s.ID, &s.Likelihood, &s.Impact, &s.RiskRating, &s.RiskLevel, &s.ColorCode); err != nil {
			return nil, fmt.Errorf("risk_score.List scan: %w", err)
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

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

type riskScoreRepository struct{ c *entityclient.Client }

// NewRiskScoreRepository creates a Compliance Entity-backed
// repository.RiskScoreRepository.
func NewRiskScoreRepository(c *entityclient.Client) repository.RiskScoreRepository {
	return &riskScoreRepository{c: c}
}

// entRiskScore is the entity's camelCase representation of a risk score.
type entRiskScore struct {
	ID         int    `json:"id"`
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskRating int    `json:"riskRating"`
	RiskLevel  string `json:"riskLevel"`
	ColorCode  string `json:"colorCode"`
}

// listRiskScoresResponse mirrors the entity's wrapper object; GET /risk/scores
// returns {"scores": [...]} rather than a bare array.
type listRiskScoresResponse struct {
	Scores []entRiskScore `json:"scores"`
}

// List returns the whole score matrix. GET /risk/scores is unpaginated — the
// matrix is fixed-size reference data — so unlike the other List methods here
// this one does not page. The entity caches the result for 30 minutes, which is
// why an entity-side change to this query needs a restart to take effect.
//
// Ordering is likelihood then impact, matching the MySQL query. The entity
// originally ordered by risk_rating, which ties and is therefore unstable; that
// was corrected in the entity rather than re-sorted here, so every future
// consumer gets the same order.
//
// The result is left nil when empty rather than an empty slice, matching the
// MySQL implementation exactly; the handler normalises nil to [] for the JSON
// response.
func (r *riskScoreRepository) List(ctx context.Context) ([]*model.RiskScore, error) {
	var resp listRiskScoresResponse
	if err := r.c.Get(ctx, "/risk/scores", &resp); err != nil {
		return nil, fmt.Errorf("list risk scores: %w", err)
	}

	var scores []*model.RiskScore
	for _, s := range resp.Scores {
		scores = append(scores, &model.RiskScore{
			ID:         s.ID,
			Likelihood: s.Likelihood,
			Impact:     s.Impact,
			RiskRating: s.RiskRating,
			RiskLevel:  s.RiskLevel,
			ColorCode:  s.ColorCode,
		})
	}
	return scores, nil
}

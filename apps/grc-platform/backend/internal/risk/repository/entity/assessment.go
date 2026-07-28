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
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type assessmentRepository struct{ c *entityclient.Client }

// NewAssessmentRepository creates a Compliance Entity-backed
// repository.RiskAssessmentRepository.
func NewAssessmentRepository(c *entityclient.Client) repository.RiskAssessmentRepository {
	return &assessmentRepository{c: c}
}

// entAssessment is the entity's camelCase representation of a risk assessment,
// including the residual score resolved from risk_score.
type entAssessment struct {
	ID                 int       `json:"id"`
	RiskID             int       `json:"riskId"`
	ScoreID            int       `json:"scoreId"`
	Progress           string    `json:"progress"`
	ReassessmentDate   string    `json:"reassessmentDate"` // YYYY-MM-DD
	AssessedBy         string    `json:"assessedBy"`
	CreatedOn          time.Time `json:"createdOn"`
	ResidualLikelihood int       `json:"residualLikelihood"`
	ResidualImpact     int       `json:"residualImpact"`
	ResidualRating     int       `json:"residualRating"`
	ResidualLevel      string    `json:"residualLevel"`
	ResidualColorCode  string    `json:"residualColorCode"`
}

type listAssessmentsResponse struct {
	Assessments []entAssessment `json:"assessments"`
}

// toModel maps an entity assessment onto the backend model.
//
// ReassessmentDate needs converting. The MySQL implementation selects the
// `date` column with parseTime=true, so the driver yields a time.Time which
// database/sql formats as RFC3339Nano when scanned into a string field — the
// API has always emitted "2026-08-31T00:00:00Z". The entity returns a plain
// "2026-08-31". Reproducing the MySQL rendering here keeps the migration
// invisible to the webapp.
//
// That rendering is arguably wrong — the webapp's parseDateOnly only accepts
// YYYY-MM-DD and returns null for it — but correcting it changes the API's
// output and belongs in its own change, not in a migration.
func (e entAssessment) toModel() model.RiskAssessment {
	return model.RiskAssessment{
		ID:                 e.ID,
		RiskID:             e.RiskID,
		ScoreID:            e.ScoreID,
		Progress:           e.Progress,
		ReassessmentDate:   dateOnlyToRFC3339(e.ReassessmentDate),
		AssessedBy:         e.AssessedBy,
		CreatedAt:          e.CreatedOn,
		ResidualLikelihood: e.ResidualLikelihood,
		ResidualImpact:     e.ResidualImpact,
		ResidualRating:     e.ResidualRating,
		ResidualLevel:      e.ResidualLevel,
		ResidualColorCode:  e.ResidualColorCode,
	}
}

// dateOnlyToRFC3339 renders a YYYY-MM-DD string the way database/sql renders a
// parsed DATE scanned into a string, so entity and MySQL output match exactly.
// Input that is not a plain date is passed through untouched rather than
// silently zeroed.
func dateOnlyToRFC3339(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// Create posts the assessment and returns it with its residual score resolved.
// Likelihood and impact go across as-is: the entity resolves them to a
// risk_score row server-side, exactly as the MySQL implementation's own lookup
// did, so this stays a single call.
func (r *assessmentRepository) Create(ctx context.Context, riskID int, req model.CreateAssessmentRequest, assessedBy string) (*model.RiskAssessment, error) {
	body := map[string]any{
		"likelihood":       req.Likelihood,
		"impact":           req.Impact,
		"progress":         req.Progress,
		"reassessmentDate": req.ReassessmentDate,
		"assessedBy":       assessedBy,
		"createdBy":        assessedBy,
	}
	var resp entAssessment
	if err := r.c.Post(ctx, fmt.Sprintf("/risks/%d/assessments", riskID), body, &resp); err != nil {
		return nil, fmt.Errorf("create assessment: %w", err)
	}
	a := resp.toModel()
	return &a, nil
}

// ListByRiskID returns a risk's assessments, newest first — the entity applies
// the same ORDER BY created_at DESC as the MySQL query.
func (r *assessmentRepository) ListByRiskID(ctx context.Context, riskID int) ([]model.RiskAssessment, error) {
	var resp listAssessmentsResponse
	if err := r.c.Get(ctx, fmt.Sprintf("/risks/%d/assessments", riskID), &resp); err != nil {
		return nil, fmt.Errorf("list assessments: %w", err)
	}

	var out []model.RiskAssessment
	for _, a := range resp.Assessments {
		out = append(out, a.toModel())
	}
	return out, nil
}

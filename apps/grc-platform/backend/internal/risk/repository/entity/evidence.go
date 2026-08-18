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

type riskEvidenceRepository struct{ c *entityclient.Client }

// NewRiskEvidenceRepository returns an entity-backed RiskEvidenceRepository.
func NewRiskEvidenceRepository(c *entityclient.Client) repository.RiskEvidenceRepository {
	return &riskEvidenceRepository{c: c}
}

// entRiskEvidence is the entity's camelCase risk evidence file.
type entRiskEvidence struct {
	ID           int       `json:"id"`
	RiskID       int       `json:"riskId"`
	ActionPlanID *int      `json:"actionPlanId"`
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	Note         *string   `json:"note"`
	EvidenceType string    `json:"evidenceType"`
	CreatedBy    *string   `json:"createdBy"`
	CreatedOn    time.Time `json:"createdOn"`
}

func (e entRiskEvidence) toModel() *model.RiskEvidence {
	m := &model.RiskEvidence{
		ID:           e.ID,
		RiskID:       e.RiskID,
		ActionPlanID: e.ActionPlanID,
		FileName:     e.FileName,
		FilePath:     e.FilePath,
		EvidenceType: e.EvidenceType,
		CreatedAt:    e.CreatedOn,
	}
	if e.Note != nil {
		m.Note = *e.Note
	}
	if e.CreatedBy != nil {
		m.CreatedBy = *e.CreatedBy
	}
	return m
}

func (r *riskEvidenceRepository) List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error) {
	var resp struct {
		Evidence []entRiskEvidence `json:"evidence"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/risks/%d/evidence", riskID), &resp); err != nil {
		return nil, fmt.Errorf("list evidence for risk %d: %w", riskID, err)
	}
	evidence := make([]*model.RiskEvidence, 0, len(resp.Evidence))
	for _, e := range resp.Evidence {
		evidence = append(evidence, e.toModel())
	}
	return evidence, nil
}

func (r *riskEvidenceRepository) GetByID(ctx context.Context, evidenceID int) (*model.RiskEvidence, error) {
	var e entRiskEvidence
	if err := r.c.Get(ctx, fmt.Sprintf("/risk-evidence/%d", evidenceID), &e); err != nil {
		return nil, fmt.Errorf("get risk evidence %d: %w", evidenceID, err)
	}
	return e.toModel(), nil
}

func (r *riskEvidenceRepository) Create(ctx context.Context, riskID int, actionPlanID *int, fileName, filePath, note, evidenceType, createdBy string) (*model.RiskEvidence, error) {
	body := map[string]any{
		"actionPlanId": actionPlanID,
		"fileName":     fileName,
		"filePath":     filePath,
		"note":         nullableString(note),
		"evidenceType": evidenceType,
		"createdBy":    createdBy,
	}
	var e entRiskEvidence
	if err := r.c.Post(ctx, fmt.Sprintf("/risks/%d/evidence", riskID), body, &e); err != nil {
		return nil, fmt.Errorf("create evidence for risk %d: %w", riskID, err)
	}
	return e.toModel(), nil
}

func (r *riskEvidenceRepository) Delete(ctx context.Context, riskID, evidenceID int) error {
	if err := r.c.Delete(ctx, fmt.Sprintf("/risks/%d/evidence/%d", riskID, evidenceID)); err != nil {
		return fmt.Errorf("delete evidence %d for risk %d: %w", evidenceID, riskID, err)
	}
	return nil
}

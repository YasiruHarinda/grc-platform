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

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

type riskEvidenceRepository struct{ db *sql.DB }

// NewRiskEvidenceRepository creates a MySQL-backed repository.RiskEvidenceRepository.
func NewRiskEvidenceRepository(db *sql.DB) repository.RiskEvidenceRepository {
	return &riskEvidenceRepository{db: db}
}

func (r *riskEvidenceRepository) List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error) {
	// TODO: implement
	return nil, nil
}

func (r *riskEvidenceRepository) Create(ctx context.Context, riskID int, fileName, filePath, note, evidenceType, createdBy string) (*model.RiskEvidence, error) {
	// TODO: INSERT INTO risk_evidence
	return nil, nil
}

func (r *riskEvidenceRepository) Delete(ctx context.Context, evidenceID int) error {
	// TODO: DELETE FROM risk_evidence WHERE id = ?
	return nil
}

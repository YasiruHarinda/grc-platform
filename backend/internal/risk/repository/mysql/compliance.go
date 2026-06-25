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

type complianceReferenceRepository struct{ db *sql.DB }

// NewComplianceReferenceRepository creates a MySQL-backed repository.ComplianceReferenceRepository.
func NewComplianceReferenceRepository(db *sql.DB) repository.ComplianceReferenceRepository {
	return &complianceReferenceRepository{db: db}
}

func (r *complianceReferenceRepository) List(ctx context.Context) ([]*model.ComplianceReference, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description
		FROM risk_security_compliance_reference
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list compliance references: %w", err)
	}
	defer rows.Close()

	var refs []*model.ComplianceReference
	for rows.Next() {
		ref := &model.ComplianceReference{}
		if err := rows.Scan(&ref.ID, &ref.Name, &ref.Description); err != nil {
			return nil, fmt.Errorf("scan compliance reference row: %w", err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (r *complianceReferenceRepository) Create(ctx context.Context, req model.CreateComplianceRefRequest, createdBy string) (*model.ComplianceReference, error) {
	// TODO: implement compliance reference INSERT
	return nil, nil
}

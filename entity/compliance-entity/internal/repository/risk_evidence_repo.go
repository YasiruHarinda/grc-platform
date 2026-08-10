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
	"errors"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskEvidenceRepository defines persistence for risk_evidence.
type RiskEvidenceRepository interface {
	CreateRiskEvidence(ctx context.Context, riskID int, req domain.CreateRiskEvidenceRequest) (*domain.RiskEvidenceFile, error)
	GetRiskEvidenceByID(ctx context.Context, fileID int) (*domain.RiskEvidenceFile, error)
	ListRiskEvidence(ctx context.Context, riskID int) (*domain.ListRiskEvidenceResponse, error)
	// DeleteRiskEvidence deletes fileID only if it belongs to riskID — a
	// mismatch behaves exactly like a missing file (404), so a caller can never
	// probe for another risk's file IDs by observing a different error.
	DeleteRiskEvidence(ctx context.Context, riskID, fileID int) error
	// HasCompletionEvidence reports whether at least one FINAL_APPROVAL_ATTACHMENT
	// row exists for actionPlanID — the gate risk_action_plan_service.go checks
	// before letting "Complete Action Plan" through.
	HasCompletionEvidence(ctx context.Context, actionPlanID int) (bool, error)
}

type riskEvidenceRepo struct{ db *sql.DB }

// NewRiskEvidenceRepository constructs a RiskEvidenceRepository.
func NewRiskEvidenceRepository(db *sql.DB) RiskEvidenceRepository {
	return &riskEvidenceRepo{db: db}
}

func (r *riskEvidenceRepo) CreateRiskEvidence(ctx context.Context, riskID int, req domain.CreateRiskEvidenceRequest) (*domain.RiskEvidenceFile, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_evidence (risk_id, action_plan_id, file_name, file_path, note, evidence_type, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		riskID, nullableInt(req.ActionPlanID), req.FileName, req.FilePath,
		nullableString(req.Note),
		req.EvidenceType, req.CreatedBy)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d (or its action plan) not found", riskID)}
		}
		return nil, fmt.Errorf("risk_evidence.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskEvidenceByID(ctx, int(id))
}

func (r *riskEvidenceRepo) GetRiskEvidenceByID(ctx context.Context, fileID int) (*domain.RiskEvidenceFile, error) {
	f, err := scanRiskEvidence(r.db.QueryRowContext(ctx,
		"SELECT id, risk_id, action_plan_id, file_name, file_path, note, evidence_type, created_by, created_at FROM risk_evidence WHERE id = ?",
		fileID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk evidence file %d not found", fileID)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_evidence.GetByID(%d): %w", fileID, err)
	}
	return f, nil
}

func (r *riskEvidenceRepo) ListRiskEvidence(ctx context.Context, riskID int) (*domain.ListRiskEvidenceResponse, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, risk_id, action_plan_id, file_name, file_path, note, evidence_type, created_by, created_at FROM risk_evidence WHERE risk_id = ? ORDER BY created_at DESC",
		riskID)
	if err != nil {
		return nil, fmt.Errorf("risk_evidence.List: %w", err)
	}
	defer rows.Close()

	var evidence []domain.RiskEvidenceFile
	for rows.Next() {
		f, err := scanRiskEvidence(rows)
		if err != nil {
			return nil, fmt.Errorf("risk_evidence.List scan: %w", err)
		}
		evidence = append(evidence, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk_evidence.List rows: %w", err)
	}
	return &domain.ListRiskEvidenceResponse{Evidence: evidence}, nil
}

func (r *riskEvidenceRepo) DeleteRiskEvidence(ctx context.Context, riskID, fileID int) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM risk_evidence WHERE id = ? AND risk_id = ?", fileID, riskID)
	if err != nil {
		return fmt.Errorf("risk_evidence.Delete(%d): %w", fileID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("risk evidence file %d not found", fileID)}
	}
	return nil
}

func (r *riskEvidenceRepo) HasCompletionEvidence(ctx context.Context, actionPlanID int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM risk_evidence
			WHERE action_plan_id = ? AND evidence_type = 'FINAL_APPROVAL_ATTACHMENT'
		)`, actionPlanID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("risk_evidence.HasCompletionEvidence(%d): %w", actionPlanID, err)
	}
	return exists, nil
}

func scanRiskEvidence(s scanner) (*domain.RiskEvidenceFile, error) {
	var f domain.RiskEvidenceFile
	var actionPlanID sql.NullInt64
	var note, createdBy sql.NullString
	err := s.Scan(&f.ID, &f.RiskID, &actionPlanID, &f.FileName, &f.FilePath, &note, &f.EvidenceType, &createdBy, &f.CreatedOn)
	if err != nil {
		return nil, err
	}
	if actionPlanID.Valid {
		v := int(actionPlanID.Int64)
		f.ActionPlanID = &v
	}
	if note.Valid {
		f.Note = &note.String
	}
	if createdBy.Valid {
		f.CreatedBy = &createdBy.String
	}
	return &f, nil
}

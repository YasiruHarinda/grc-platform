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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskReferenceRepository defines persistence operations for risk_security_compliance_reference.
type RiskReferenceRepository interface {
	SearchRiskReferences(ctx context.Context, req domain.SearchRiskReferencesRequest) ([]domain.RiskComplianceReference, int, error)
	GetRiskReferenceByID(ctx context.Context, id int) (*domain.RiskComplianceReference, error)
	CreateRiskReference(ctx context.Context, req domain.CreateRiskReferenceRequest) (*domain.RiskComplianceReference, error)
	UpdateRiskReference(ctx context.Context, id int, req domain.UpdateRiskReferenceRequest) (*domain.RiskComplianceReference, error)
	DeleteRiskReference(ctx context.Context, id int) error
}

type riskReferenceRepo struct{ db *sql.DB }

// NewRiskReferenceRepository constructs a RiskReferenceRepository.
func NewRiskReferenceRepository(db *sql.DB) RiskReferenceRepository {
	return &riskReferenceRepo{db: db}
}

func (r *riskReferenceRepo) SearchRiskReferences(ctx context.Context, req domain.SearchRiskReferencesRequest) ([]domain.RiskComplianceReference, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+likeEscape(req.SearchQuery)+"%")
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM risk_security_compliance_reference "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("risk_reference.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, description, created_at, updated_at "+
			"FROM risk_security_compliance_reference "+where+" ORDER BY name LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("risk_reference.Search query: %w", err)
	}
	defer rows.Close()

	var refs []domain.RiskComplianceReference
	for rows.Next() {
		ref, err := scanRiskReference(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("risk_reference.Search scan: %w", err)
		}
		refs = append(refs, *ref)
	}
	return refs, total, rows.Err()
}

func (r *riskReferenceRepo) GetRiskReferenceByID(ctx context.Context, id int) (*domain.RiskComplianceReference, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, description, created_at, updated_at FROM risk_security_compliance_reference WHERE id = ?", id)
	ref, err := scanRiskReference(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("compliance reference %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_reference.GetByID(%d): %w", id, err)
	}
	return ref, nil
}

func (r *riskReferenceRepo) CreateRiskReference(ctx context.Context, req domain.CreateRiskReferenceRequest) (*domain.RiskComplianceReference, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO risk_security_compliance_reference (name, description, created_by, updated_by) VALUES (?, ?, ?, ?)",
		req.Name, nullableString(req.Description), req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("risk_reference.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskReferenceByID(ctx, int(id))
}

func (r *riskReferenceRepo) UpdateRiskReference(ctx context.Context, id int, req domain.UpdateRiskReferenceRequest) (*domain.RiskComplianceReference, error) {
	sets := []string{}
	args := []any{}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, id)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE risk_security_compliance_reference SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("risk_reference.Update(%d): %w", id, err)
	}
	return r.GetRiskReferenceByID(ctx, id)
}

// DeleteRiskReference removes a compliance reference outright — no status
// column here either (same as risk_category — see its DeleteRiskCategory for
// the fuller reasoning). risk_compliance_reference.reference_id has ON DELETE
// CASCADE back to this table, so the in-use count is checked explicitly
// before the DELETE rather than relying on a constraint error that MySQL will
// never raise.
func (r *riskReferenceRepo) DeleteRiskReference(ctx context.Context, id int) error {
	var inUse int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM risk_compliance_reference WHERE reference_id = ?", id).Scan(&inUse); err != nil {
		return fmt.Errorf("risk_reference.Delete(%d) in-use check: %w", id, err)
	}
	if inUse > 0 {
		return &apierror.ConflictError{
			Msg: fmt.Sprintf("compliance reference is used by %d risk(s) and cannot be deleted", inUse)}
	}

	res, err := r.db.ExecContext(ctx, "DELETE FROM risk_security_compliance_reference WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("risk_reference.Delete(%d): %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("compliance reference %d not found", id)}
	}
	return nil
}

func scanRiskReference(s scanner) (*domain.RiskComplianceReference, error) {
	var ref domain.RiskComplianceReference
	var description sql.NullString
	if err := s.Scan(&ref.ID, &ref.Name, &description, &ref.CreatedOn, &ref.UpdatedOn); err != nil {
		return nil, err
	}
	if description.Valid {
		ref.Description = &description.String
	}
	return &ref, nil
}

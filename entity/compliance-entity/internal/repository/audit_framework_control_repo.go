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

// FrameworkControlRepository defines persistence operations for audit_framework_control.
type FrameworkControlRepository interface {
	ListCurrentControls(ctx context.Context, frameworkID int) ([]domain.AuditFrameworkControl, error)
	ListAllVersions(ctx context.Context, frameworkID int, controlNumber string) ([]domain.AuditFrameworkControl, error)
	GetByID(ctx context.Context, id int) (*domain.AuditFrameworkControl, error)
	// GetCurrentByNumber returns the current (is_current=TRUE) row for a control
	// number within a framework, else sql.ErrNoRows. Used to detect a collision
	// when a caller tries to push a brand-new control into the catalog under a
	// number that's already there.
	GetCurrentByNumber(ctx context.Context, frameworkID int, controlNumber string) (*domain.AuditFrameworkControl, error)
	Create(ctx context.Context, frameworkID int, req domain.CreateFrameworkControlRequest) (*domain.AuditFrameworkControl, error)
	// NewVersion creates a new version row for an existing control and marks the
	// previous is_current row as superseded. Returns the new row.
	NewVersion(ctx context.Context, id int, req domain.UpdateFrameworkControlRequest) (*domain.AuditFrameworkControl, error)
}

type frameworkControlRepo struct{ db *sql.DB }

// NewFrameworkControlRepository constructs a FrameworkControlRepository.
func NewFrameworkControlRepository(db *sql.DB) FrameworkControlRepository {
	return &frameworkControlRepo{db: db}
}

const fwCtlCols = `id, framework_id, control_number, description, evidence_requirement,
	requirement_type, control_type, scope, version, is_current, created_at, created_by`

func (r *frameworkControlRepo) ListCurrentControls(ctx context.Context, frameworkID int) ([]domain.AuditFrameworkControl, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+fwCtlCols+" FROM audit_framework_control WHERE framework_id = ? AND is_current = TRUE ORDER BY control_number",
		frameworkID)
	if err != nil {
		return nil, fmt.Errorf("framework_control.ListCurrent(%d): %w", frameworkID, err)
	}
	defer rows.Close()
	return scanFWControls(rows)
}

func (r *frameworkControlRepo) ListAllVersions(ctx context.Context, frameworkID int, controlNumber string) ([]domain.AuditFrameworkControl, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+fwCtlCols+" FROM audit_framework_control WHERE framework_id = ? AND control_number = ? ORDER BY version DESC",
		frameworkID, controlNumber)
	if err != nil {
		return nil, fmt.Errorf("framework_control.ListVersions(%d,%s): %w", frameworkID, controlNumber, err)
	}
	defer rows.Close()
	return scanFWControls(rows)
}

func (r *frameworkControlRepo) GetByID(ctx context.Context, id int) (*domain.AuditFrameworkControl, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+fwCtlCols+" FROM audit_framework_control WHERE id = ?", id)
	c, err := scanFWControl(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("framework control %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("framework_control.GetByID(%d): %w", id, err)
	}
	return c, nil
}

func (r *frameworkControlRepo) GetCurrentByNumber(ctx context.Context, frameworkID int, controlNumber string) (*domain.AuditFrameworkControl, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+fwCtlCols+" FROM audit_framework_control WHERE framework_id = ? AND control_number = ? AND is_current = TRUE",
		frameworkID, controlNumber)
	return scanFWControl(row)
}

func (r *frameworkControlRepo) Create(ctx context.Context, frameworkID int, req domain.CreateFrameworkControlRequest) (*domain.AuditFrameworkControl, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_framework_control
		  (framework_id, control_number, description, evidence_requirement,
		   requirement_type, control_type, scope, version, is_current, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, TRUE, ?)`,
		frameworkID, req.ControlNumber, req.Description,
		nullableString(req.EvidenceRequirement),
		req.RequirementType, req.ControlType, req.Scope, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("framework_control.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetByID(ctx, int(id))
}

func (r *frameworkControlRepo) NewVersion(ctx context.Context, id int, req domain.UpdateFrameworkControlRequest) (*domain.AuditFrameworkControl, error) {
	old, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !old.IsCurrent {
		return nil, &apierror.ValidationError{Msg: "cannot version a superseded control; use the current version id"}
	}

	// Apply partial updates on top of the existing definition.
	description := old.Description
	if req.Description != nil {
		description = *req.Description
	}
	evidenceReq := old.EvidenceRequirement
	if req.EvidenceRequirement != nil {
		evidenceReq = req.EvidenceRequirement
	}
	requirementType := old.RequirementType
	if req.RequirementType != nil {
		requirementType = *req.RequirementType
	}
	controlType := old.ControlType
	if req.ControlType != nil {
		controlType = *req.ControlType
	}
	scope := old.Scope
	if req.Scope != nil {
		scope = *req.Scope
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("framework_control.NewVersion: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Supersede old row first, guarded by is_current = TRUE.
	// Two concurrent NewVersion calls on the same id will both attempt this UPDATE;
	// only one can match (RowsAffected = 1), the other gets 0 → ConflictError,
	// preventing duplicate is_current rows.
	supResult, err := tx.ExecContext(ctx,
		"UPDATE audit_framework_control SET is_current = FALSE WHERE id = ? AND is_current = TRUE", id)
	if err != nil {
		return nil, fmt.Errorf("framework_control.NewVersion: supersede old: %w", err)
	}
	if n, _ := supResult.RowsAffected(); n == 0 {
		return nil, &apierror.ConflictError{Msg: "framework control was already versioned concurrently, please retry"}
	}

	// Insert new version row.
	res, err := tx.ExecContext(ctx, `
		INSERT INTO audit_framework_control
		  (framework_id, control_number, description, evidence_requirement,
		   requirement_type, control_type, scope, version, is_current, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, TRUE, ?)`,
		old.FrameworkID, old.ControlNumber, description,
		nullableString(evidenceReq),
		requirementType, controlType, scope, old.Version+1, req.UpdatedBy)
	if err != nil {
		return nil, fmt.Errorf("framework_control.NewVersion: insert: %w", err)
	}
	newID, _ := res.LastInsertId()

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("framework_control.NewVersion: commit: %w", err)
	}
	return r.GetByID(ctx, int(newID))
}

func scanFWControls(rows *sql.Rows) ([]domain.AuditFrameworkControl, error) {
	var out []domain.AuditFrameworkControl
	for rows.Next() {
		c, err := scanFWControl(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func scanFWControl(s scanner) (*domain.AuditFrameworkControl, error) {
	var c domain.AuditFrameworkControl
	var evidenceReq, createdBy sql.NullString
	if err := s.Scan(
		&c.ID, &c.FrameworkID, &c.ControlNumber, &c.Description, &evidenceReq,
		&c.RequirementType, &c.ControlType, &c.Scope,
		&c.Version, &c.IsCurrent, &c.CreatedOn, &createdBy,
	); err != nil {
		return nil, err
	}
	if evidenceReq.Valid {
		c.EvidenceRequirement = &evidenceReq.String
	}
	if createdBy.Valid {
		c.CreatedBy = &createdBy.String
	}
	return &c, nil
}

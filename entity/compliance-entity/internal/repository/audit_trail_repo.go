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

// AuditTrailRepository defines persistence for audit_trail.
type AuditTrailRepository interface {
	CreateAuditTrail(ctx context.Context, auditID int, req domain.CreateAuditTrailRequest) (*domain.AuditTrail, error)
	// ListAuditTrail returns the audit's trail, newest first. When controlID is non-nil
	// the result is narrowed to that control (used by the per-control History view).
	ListAuditTrail(ctx context.Context, auditID int, controlID *int, limit, offset int) ([]domain.AuditTrail, int, error)
}

type auditTrailRepo struct{ db *sql.DB }

// NewAuditTrailRepository constructs a AuditTrailRepository.
func NewAuditTrailRepository(db *sql.DB) AuditTrailRepository { return &auditTrailRepo{db: db} }

func (r *auditTrailRepo) CreateAuditTrail(ctx context.Context, auditID int, req domain.CreateAuditTrailRequest) (*domain.AuditTrail, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_trail
		 (audit_id, actor_id, control_id, evidence_id, action, details, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		auditID,
		nullableInt(req.ActorID),
		nullableInt(req.ControlID),
		nullableInt(req.EvidenceID),
		req.Action,
		nullableString(req.Details),
		nullableString(req.CreatedBy),
	)
	if err != nil {
		return nil, fmt.Errorf("audit_trail.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("audit_trail.Create last insert id: %w", err)
	}
	return r.getAuditTrailByID(ctx, id)
}

func (r *auditTrailRepo) getAuditTrailByID(ctx context.Context, id int64) (*domain.AuditTrail, error) {
	return scanAuditTrail(r.db.QueryRowContext(ctx,
		`SELECT id, actor_id, audit_id, control_id, evidence_id, action,
		        details, created_by, created_at
		 FROM audit_trail WHERE id = ?`, id))
}

func (r *auditTrailRepo) ListAuditTrail(ctx context.Context, auditID int, controlID *int, limit, offset int) ([]domain.AuditTrail, int, error) {
	// Optional control narrowing: `AND (? IS NULL OR control_id = ?)` keeps a single
	// query for both the audit-wide and per-control cases. controlArg is passed twice.
	var controlArg any
	if controlID != nil {
		controlArg = *controlID
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_trail
		 WHERE audit_id = ? AND (? IS NULL OR control_id = ?)`,
		auditID, controlArg, controlArg).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_trail.ListCount: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, actor_id, audit_id, control_id, evidence_id, action,
		        details, created_by, created_at
		 FROM audit_trail
		 WHERE audit_id = ? AND (? IS NULL OR control_id = ?)
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		auditID, controlArg, controlArg, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_trail.List: %w", err)
	}
	defer rows.Close()

	var entries []domain.AuditTrail
	for rows.Next() {
		e, err := scanAuditTrail(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("audit_trail.List scan: %w", err)
		}
		entries = append(entries, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("audit_trail.List rows: %w", err)
	}
	return entries, total, nil
}

func scanAuditTrail(s scanner) (*domain.AuditTrail, error) {
	var e domain.AuditTrail
	var actorID, controlID, evidenceID sql.NullInt64
	var details, createdBy sql.NullString
	err := s.Scan(
		&e.ID, &actorID, &e.AuditID, &controlID, &evidenceID,
		&e.Action, &details, &createdBy, &e.CreatedOn,
	)
	if err != nil {
		return nil, err
	}
	if actorID.Valid {
		v := int(actorID.Int64)
		e.ActorID = &v
	}
	if controlID.Valid {
		v := int(controlID.Int64)
		e.ControlID = &v
	}
	if evidenceID.Valid {
		v := int(evidenceID.Int64)
		e.EvidenceID = &v
	}
	if details.Valid {
		e.Details = &details.String
	}
	if createdBy.Valid {
		e.CreatedBy = &createdBy.String
	}
	return &e, nil
}

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
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// AuditTrailRepository defines persistence for audit_trail.
type AuditTrailRepository interface {
	CreateAuditTrail(ctx context.Context, auditID int, req domain.CreateAuditTrailRequest) (*domain.AuditTrail, error)
	// ListAuditTrail returns the audit's trail, newest first, narrowed by filter
	// (all fields optional). filter.ControlID non-nil is used by the per-control
	// History view; filter left zero-value returns the whole audit's trail, used
	// by the audit-wide activity log.
	ListAuditTrail(ctx context.Context, auditID int, filter domain.TrailFilter, limit, offset int) ([]domain.AuditTrail, int, error)
}

type auditTrailRepo struct{ db *sql.DB }

// NewAuditTrailRepository constructs a AuditTrailRepository.
func NewAuditTrailRepository(db *sql.DB) AuditTrailRepository { return &auditTrailRepo{db: db} }

// nilableAny converts a typed pointer to `any`, nil-preserving, for use as a
// driver arg in an `(? IS NULL OR col = ?)` clause — the driver needs a plain
// nil (not a nil *int/*string/*time.Time) to bind SQL NULL correctly.
func nilableAny[T any](v *T) any {
	if v == nil {
		return nil
	}
	return *v
}

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

const auditTrailSelectCols = `
	        t.id, t.actor_id, t.audit_id, t.control_id, t.evidence_id, t.action,
	        t.details, t.created_by, u_actor.user_type AS actor_user_type, t.created_at`

const auditTrailFromClause = `
	 FROM audit_trail t
	 LEFT JOIN ` + "`user`" + ` u_actor ON u_actor.id = t.actor_id`

func (r *auditTrailRepo) getAuditTrailByID(ctx context.Context, id int64) (*domain.AuditTrail, error) {
	return scanAuditTrail(r.db.QueryRowContext(ctx,
		`SELECT`+auditTrailSelectCols+auditTrailFromClause+` WHERE t.id = ?`, id))
}

// inClause returns "col IN (?,?,...)" and the values as `any`, or ("", nil) when
// values is empty (caller skips the clause entirely — no filter).
func inClause[T any](col string, values []T) (string, []any) {
	if len(values) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args[i] = v
	}
	return col + " IN (" + strings.Join(placeholders, ",") + ")", args
}

// auditTrailScopeWhere mirrors controlScopeWhere, for use inside an EXISTS(...)
// subquery against audit_control aliased "c"; ("", nil) for ScopeAll.
func auditTrailScopeWhere(scope domain.Scope, userID int, scopeTeamIDs []int) (string, []any) {
	switch scope {
	case domain.ScopeAll:
		return "", nil
	case domain.ScopeOwned:
		return " AND c.owner_id = ?", []any{userID}
	case domain.ScopeAssigned:
		return " AND c.auditor_id = ?", []any{userID}
	case domain.ScopeTeam:
		pred, args := teamScopePredicate("c", scopeTeamIDs, userID)
		return " AND " + pred, args
	default: // ScopeNone and any unrecognized value scope to nothing.
		return " AND 1=0", nil
	}
}

func (r *auditTrailRepo) ListAuditTrail(ctx context.Context, auditID int, filter domain.TrailFilter, limit, offset int) ([]domain.AuditTrail, int, error) {
	// Control filter uses IN (...), built only when non-empty. Date range keeps
	// the fixed `(? IS NULL OR col >= /<= ?)` shape so one query serves both the
	// audit-wide (all filters empty) and narrowed cases.
	where := "WHERE audit_id = ?"
	args := []any{auditID}

	if clause, clauseArgs := inClause("control_id", filter.ControlIDs); clause != "" {
		where += " AND " + clause
		args = append(args, clauseArgs...)
	}
	// Audit-level rows (control_id IS NULL) always pass; control-level rows
	// require the control itself to be in the caller's scope.
	if scopeClause, scopeArgs := auditTrailScopeWhere(filter.Scope, filter.UserID, filter.ScopeTeamIDs); scopeClause != "" {
		where += " AND (t.control_id IS NULL OR EXISTS (SELECT 1 FROM audit_control c WHERE c.id = t.control_id" + scopeClause + "))"
		args = append(args, scopeArgs...)
	}
	where += " AND (? IS NULL OR t.created_at >= ?) AND (? IS NULL OR t.created_at <= ?)"
	args = append(args,
		nilableAny(filter.From), nilableAny(filter.From),
		nilableAny(filter.To), nilableAny(filter.To),
	)
	if !filter.IncludeInternal {
		// Fail closed: visible only when isInternal decodes to exactly JSON false.
		where += ` AND (t.action != 'COMMENTED' OR JSON_EXTRACT(t.details, '$.isInternal') = CAST('false' AS JSON))`
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_trail t "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_trail.ListCount: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT`+auditTrailSelectCols+auditTrailFromClause+" "+where+
			" ORDER BY t.created_at DESC LIMIT ? OFFSET ?",
		append(append([]any{}, args...), limit, offset)...)
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
	var details, createdBy, actorUserType sql.NullString
	err := s.Scan(
		&e.ID, &actorID, &e.AuditID, &controlID, &evidenceID,
		&e.Action, &details, &createdBy, &actorUserType, &e.CreatedOn,
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
	if actorUserType.Valid {
		e.CreatedByUserType = &actorUserType.String
	}
	return &e, nil
}

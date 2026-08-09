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

// AuditRepository defines persistence operations for the audit table.
type AuditRepository interface {
	SearchAudits(ctx context.Context, req domain.SearchAuditsRequest) ([]domain.Audit, int, error)
	GetAuditByID(ctx context.Context, id int) (*domain.Audit, error)
	CreateAudit(ctx context.Context, req domain.CreateAuditRequest) (*domain.Audit, error)
	UpdateAudit(ctx context.Context, id int, req domain.UpdateAuditRequest) (*domain.Audit, error)
	DeleteAudit(ctx context.Context, id int, deletedBy string) error
}

type auditRepo struct{ db *sql.DB }

// NewAuditRepository constructs an AuditRepository.
func NewAuditRepository(db *sql.DB) AuditRepository { return &auditRepo{db: db} }

const auditSelectCols = `
  a.id, a.name,
  a.framework_id, f.name AS framework_name,
  a.product_id,   p.name AS product_name,
  DATE_FORMAT(a.period_start, '%Y-%m-%d'), DATE_FORMAT(a.period_end, '%Y-%m-%d'),
  a.status, a.scope_description,
  a.created_at, a.updated_at,
  (SELECT COUNT(*) FROM audit_control cc WHERE cc.audit_id = a.id) AS controls_total,
  (SELECT COUNT(*) FROM audit_control cc WHERE cc.audit_id = a.id AND cc.status = 'COMPLETE') AS controls_approved,
  (SELECT COUNT(*) FROM audit_control cc WHERE cc.audit_id = a.id
     AND cc.due_date IS NOT NULL AND cc.due_date < CURDATE() AND cc.status <> 'COMPLETE') AS controls_overdue`

const auditFromClause = `
FROM audit a
JOIN audit_framework f ON f.id = a.framework_id
JOIN audit_product   p ON p.id = a.product_id`

// auditScopeWhere returns a WHERE fragment (starting with "AND") and its bind
// args for the given row scope. Audits have no team/owner/auditor of their own
// (only their controls do), so — per the same rule the dashboard applies at the
// control level (see audit_dashboard_repo.go's scopeWhere) — an audit is in
// scope when it has at least one control in scope, checked via EXISTS against
// audit_control c.
func auditScopeWhere(scope domain.Scope, userEmail string) (string, []any) {
	switch scope {
	case "", domain.ScopeAll:
		return "", nil
	case domain.ScopeOwnTeam:
		return ` AND EXISTS (SELECT 1 FROM audit_control c WHERE c.audit_id = a.id
			AND c.team_id IN (SELECT uat.audit_team_id FROM user_audit_team uat JOIN ` + "`user`" + ` u ON u.id = uat.user_id WHERE u.email = ? AND uat.is_active = TRUE))`,
			[]any{userEmail}
	case domain.ScopeOwned:
		return " AND EXISTS (SELECT 1 FROM audit_control c WHERE c.audit_id = a.id AND c.owner_id = (SELECT id FROM `user` WHERE email = ?))",
			[]any{userEmail}
	case domain.ScopeAssigned:
		return " AND EXISTS (SELECT 1 FROM audit_control c WHERE c.audit_id = a.id AND c.auditor_id = (SELECT id FROM `user` WHERE email = ?))",
			[]any{userEmail}
	default: // ScopeNone and any unrecognized value scope to nothing.
		return " AND 1=0", nil
	}
}

func (r *auditRepo) SearchAudits(ctx context.Context, req domain.SearchAuditsRequest) ([]domain.Audit, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND a.name LIKE ?"
		args = append(args, "%"+likeEscape(req.SearchQuery)+"%")
	}
	if len(req.StatusKeys) > 0 {
		placeholders := strings.Repeat("?,", len(req.StatusKeys))
		placeholders = placeholders[:len(placeholders)-1]
		where += " AND a.status IN (" + placeholders + ")"
		for _, s := range req.StatusKeys {
			args = append(args, s)
		}
	} else {
		// Hide soft-deleted audits by default; callers can request REMOVED explicitly.
		where += " AND a.status != 'REMOVED'"
	}
	if len(req.FrameworkIDs) > 0 {
		placeholders := strings.Repeat("?,", len(req.FrameworkIDs))
		placeholders = placeholders[:len(placeholders)-1]
		where += " AND a.framework_id IN (" + placeholders + ")"
		for _, id := range req.FrameworkIDs {
			args = append(args, id)
		}
	}
	if len(req.ProductIDs) > 0 {
		placeholders := strings.Repeat("?,", len(req.ProductIDs))
		placeholders = placeholders[:len(placeholders)-1]
		where += " AND a.product_id IN (" + placeholders + ")"
		for _, id := range req.ProductIDs {
			args = append(args, id)
		}
	}
	if len(req.AuditIDs) > 0 {
		placeholders := strings.Repeat("?,", len(req.AuditIDs))
		placeholders = placeholders[:len(placeholders)-1]
		where += " AND a.id IN (" + placeholders + ")"
		for _, id := range req.AuditIDs {
			args = append(args, id)
		}
	}
	scopeClause, scopeArgs := auditScopeWhere(req.Scope, req.UserEmail)
	where += scopeClause
	args = append(args, scopeArgs...)

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+auditFromClause+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+auditSelectCols+auditFromClause+" "+where+" ORDER BY a.created_at DESC LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit.Search query: %w", err)
	}
	defer rows.Close()

	var audits []domain.Audit
	for rows.Next() {
		a, err := scanAudit(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("audit.Search scan: %w", err)
		}
		audits = append(audits, *a)
	}
	return audits, total, rows.Err()
}

func (r *auditRepo) GetAuditByID(ctx context.Context, id int) (*domain.Audit, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT"+auditSelectCols+auditFromClause+" WHERE a.id = ?", id)
	a, err := scanAudit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("audit %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("audit.GetByID(%d): %w", id, err)
	}
	return a, nil
}

func (r *auditRepo) CreateAudit(ctx context.Context, req domain.CreateAuditRequest) (*domain.Audit, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit (name, framework_id, product_id, period_start, period_end, scope_description, status, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE', ?, ?)`,
		req.Name, req.FrameworkID, req.ProductID,
		req.PeriodStart, req.PeriodEnd,
		nullableString(req.ScopeDescription),
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("audit.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetAuditByID(ctx, int(id))
}

func (r *auditRepo) UpdateAudit(ctx context.Context, id int, req domain.UpdateAuditRequest) (*domain.Audit, error) {
	sets := []string{}
	args := []any{}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	if req.PeriodStart != nil {
		sets = append(sets, "period_start = ?")
		args = append(args, *req.PeriodStart)
	}
	if req.PeriodEnd != nil {
		sets = append(sets, "period_end = ?")
		args = append(args, *req.PeriodEnd)
	}
	if req.ScopeDescription != nil {
		sets = append(sets, "scope_description = ?")
		args = append(args, *req.ScopeDescription)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, id)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE audit SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("audit.Update(%d): %w", id, err)
	}
	return r.GetAuditByID(ctx, id)
}

func (r *auditRepo) DeleteAudit(ctx context.Context, id int, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE audit SET status = 'REMOVED', updated_by = ? WHERE id = ? AND status != 'REMOVED'",
		deletedBy, id)
	if err != nil {
		return fmt.Errorf("audit.Delete(%d): %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("audit %d not found", id)}
	}
	return nil
}

// scanner is the common interface satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanAudit(s scanner) (*domain.Audit, error) {
	var a domain.Audit
	var scopeDesc sql.NullString
	err := s.Scan(
		&a.ID, &a.Name,
		&a.FrameworkID, &a.FrameworkName,
		&a.ProductID, &a.ProductName,
		&a.PeriodStart, &a.PeriodEnd,
		&a.Status, &scopeDesc,
		&a.CreatedOn, &a.UpdatedOn,
		&a.ControlsTotal, &a.ControlsApproved, &a.ControlsOverdue,
	)
	if err != nil {
		return nil, err
	}
	if scopeDesc.Valid {
		a.ScopeDescription = &scopeDesc.String
	}
	return &a, nil
}

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

// AuditFrameworkRepository defines persistence operations for the audit_framework table.
type AuditFrameworkRepository interface {
	SearchAuditFrameworks(ctx context.Context, req domain.SearchAuditFrameworksRequest) ([]domain.AuditFramework, int, error)
	GetAuditFrameworkByID(ctx context.Context, id int) (*domain.AuditFramework, error)
	CreateAuditFramework(ctx context.Context, req domain.CreateAuditFrameworkRequest) (*domain.AuditFramework, error)
	UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (*domain.AuditFramework, error)
}

type auditFrameworkRepo struct{ db *sql.DB }

// NewAuditFrameworkRepository constructs an AuditFrameworkRepository.
func NewAuditFrameworkRepository(db *sql.DB) AuditFrameworkRepository {
	return &auditFrameworkRepo{db: db}
}

func (r *auditFrameworkRepo) SearchAuditFrameworks(ctx context.Context, req domain.SearchAuditFrameworksRequest) ([]domain.AuditFramework, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND name LIKE ?"
		args = append(args, "%"+likeEscape(req.SearchQuery)+"%")
	}
	if req.StatusKey != "" {
		where += " AND status = ?"
		args = append(args, req.StatusKey)
	}
	scopeClause, scopeArgs := frameworkScopeWhere(req.Scope, req.UserEmail)
	where += scopeClause
	args = append(args, scopeArgs...)

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_framework "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_framework.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_framework "+where+" ORDER BY name LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_framework.Search query: %w", err)
	}
	defer rows.Close()

	var frameworks []domain.AuditFramework
	for rows.Next() {
		f, err := scanFramework(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("audit_framework.Search scan: %w", err)
		}
		frameworks = append(frameworks, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("audit_framework.Search rows: %w", err)
	}
	return frameworks, total, nil
}

func (r *auditFrameworkRepo) GetAuditFrameworkByID(ctx context.Context, id int) (*domain.AuditFramework, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_framework WHERE id = ?", id)
	f, err := scanFramework(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("framework %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("audit_framework.GetByID(%d): %w", id, err)
	}
	return f, nil
}

func (r *auditFrameworkRepo) CreateAuditFramework(ctx context.Context, req domain.CreateAuditFrameworkRequest) (*domain.AuditFramework, error) {
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO audit_framework (name, status, created_by, updated_by) VALUES (?, ?, ?, ?)",
		req.Name, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("audit_framework.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetAuditFrameworkByID(ctx, int(id))
}

func (r *auditFrameworkRepo) UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (*domain.AuditFramework, error) {
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
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, id)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE audit_framework SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("audit_framework.Update(%d): %w", id, err)
	}
	return r.GetAuditFrameworkByID(ctx, id)
}

// frameworkScopeWhere returns a WHERE fragment (starting with "AND") and its
// bind args for the given row scope. Frameworks have no team/owner/auditor of
// their own (only their audits' controls do), so — one level deeper than
// auditScopeWhere's "audit qualifies if any control does" — a framework
// qualifies if any of its audits has any control in scope.
func frameworkScopeWhere(scope domain.Scope, userEmail string) (string, []any) {
	switch scope {
	case "", domain.ScopeAll:
		return "", nil
	case domain.ScopeOwned:
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.framework_id = audit_framework.id AND c.owner_id = (SELECT id FROM ` + "`user`" + ` WHERE email = ?))`,
			[]any{userEmail}
	case domain.ScopeAssigned:
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.framework_id = audit_framework.id AND c.auditor_id = (SELECT id FROM ` + "`user`" + ` WHERE email = ?))`,
			[]any{userEmail}
	default: // ScopeNone and any unrecognized value scope to nothing.
		return " AND 1=0", nil
	}
}

func scanFramework(s scanner) (*domain.AuditFramework, error) {
	var f domain.AuditFramework
	if err := s.Scan(&f.ID, &f.Name, &f.Status, &f.CreatedOn, &f.UpdatedOn); err != nil {
		return nil, err
	}
	return &f, nil
}

// nullableString converts *string to sql.NullString for optional columns.
func nullableString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

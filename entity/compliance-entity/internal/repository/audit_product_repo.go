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

// AuditProductRepository defines persistence operations for the audit_product table.
type AuditProductRepository interface {
	SearchAuditProducts(ctx context.Context, req domain.SearchAuditProductsRequest) ([]domain.AuditProduct, int, error)
	GetAuditProductByID(ctx context.Context, id int) (*domain.AuditProduct, error)
	CreateAuditProduct(ctx context.Context, req domain.CreateAuditProductRequest) (*domain.AuditProduct, error)
	UpdateAuditProduct(ctx context.Context, id int, req domain.UpdateAuditProductRequest) (*domain.AuditProduct, error)
}

type auditProductRepo struct{ db *sql.DB }

// NewAuditProductRepository constructs an AuditProductRepository.
func NewAuditProductRepository(db *sql.DB) AuditProductRepository {
	return &auditProductRepo{db: db}
}

func (r *auditProductRepo) SearchAuditProducts(ctx context.Context, req domain.SearchAuditProductsRequest) ([]domain.AuditProduct, int, error) {
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
	scopeClause, scopeArgs := productScopeWhere(req.Scope, req.UserID, req.ScopeTeamIDs)
	where += scopeClause
	args = append(args, scopeArgs...)

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_product "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_product.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_product "+where+" ORDER BY name LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_product.Search query: %w", err)
	}
	defer rows.Close()

	var products []domain.AuditProduct
	for rows.Next() {
		var p domain.AuditProduct
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.CreatedOn, &p.UpdatedOn); err != nil {
			return nil, 0, fmt.Errorf("audit_product.Search scan: %w", err)
		}
		products = append(products, p)
	}
	return products, total, rows.Err()
}

// productScopeWhere mirrors frameworkScopeWhere: a product has no team/owner
// of its own, so it qualifies when one of its audits has a control in scope.
func productScopeWhere(scope domain.Scope, userID int, scopeTeamIDs []int) (string, []any) {
	switch scope {
	case domain.ScopeAll:
		return "", nil
	case domain.ScopeOwned:
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.product_id = audit_product.id AND c.owner_id = ?)`,
			[]any{userID}
	case domain.ScopeAssigned:
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.product_id = audit_product.id AND c.auditor_id = ?)`,
			[]any{userID}
	case domain.ScopeTeam:
		pred, args := teamScopePredicate("c", scopeTeamIDs, userID)
		return ` AND EXISTS (SELECT 1 FROM audit a JOIN audit_control c ON c.audit_id = a.id
			WHERE a.product_id = audit_product.id AND ` + pred + `)`,
			args
	default: // ScopeNone and any unrecognized value scope to nothing.
		return " AND 1=0", nil
	}
}

func (r *auditProductRepo) GetAuditProductByID(ctx context.Context, id int) (*domain.AuditProduct, error) {
	var p domain.AuditProduct
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, status, created_at, updated_at FROM audit_product WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Status, &p.CreatedOn, &p.UpdatedOn)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("product %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("audit_product.GetByID(%d): %w", id, err)
	}
	return &p, nil
}

func (r *auditProductRepo) CreateAuditProduct(ctx context.Context, req domain.CreateAuditProductRequest) (*domain.AuditProduct, error) {
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO audit_product (name, status, created_by, updated_by) VALUES (?, ?, ?, ?)",
		req.Name, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("audit_product.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetAuditProductByID(ctx, int(id))
}

func (r *auditProductRepo) UpdateAuditProduct(ctx context.Context, id int, req domain.UpdateAuditProductRequest) (*domain.AuditProduct, error) {
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
		"UPDATE audit_product SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("audit_product.Update(%d): %w", id, err)
	}
	return r.GetAuditProductByID(ctx, id)
}

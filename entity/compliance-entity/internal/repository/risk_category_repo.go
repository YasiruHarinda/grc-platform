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

// RiskCategoryRepository defines persistence operations for the risk_category table.
type RiskCategoryRepository interface {
	ListRiskCategories(ctx context.Context) ([]domain.RiskCategory, error)
	GetRiskCategoryByID(ctx context.Context, id int) (*domain.RiskCategory, error)
	CreateRiskCategory(ctx context.Context, req domain.CreateRiskCategoryRequest) (*domain.RiskCategory, error)
	UpdateRiskCategory(ctx context.Context, id int, req domain.UpdateRiskCategoryRequest) (*domain.RiskCategory, error)
	DeleteRiskCategory(ctx context.Context, id int) error
}

type riskCategoryRepo struct{ db *sql.DB }

// NewRiskCategoryRepository constructs a RiskCategoryRepository.
func NewRiskCategoryRepository(db *sql.DB) RiskCategoryRepository { return &riskCategoryRepo{db: db} }

// ListRiskCategories returns every seeded risk category ordered by name.
func (r *riskCategoryRepo) ListRiskCategories(ctx context.Context) ([]domain.RiskCategory, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, description FROM risk_category ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("risk_category.List: %w", err)
	}
	defer rows.Close()

	var cats []domain.RiskCategory
	for rows.Next() {
		c, err := scanRiskCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("risk_category.List scan: %w", err)
		}
		cats = append(cats, *c)
	}
	return cats, rows.Err()
}

func (r *riskCategoryRepo) GetRiskCategoryByID(ctx context.Context, id int) (*domain.RiskCategory, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, name, description FROM risk_category WHERE id = ?", id)
	c, err := scanRiskCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk category %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_category.GetByID(%d): %w", id, err)
	}
	return c, nil
}

// CreateRiskCategory inserts a new category. uq_risk_category_name already
// enforces uniqueness at the DB level (see risk_schema.sql — the seed file's
// own INSERT IGNORE relies on it); this maps that constraint to a clean 409
// instead of letting the raw MySQL duplicate-key error bubble up, matching
// the pattern already used for audit control numbers (see
// audit_control_repo.go).
func (r *riskCategoryRepo) CreateRiskCategory(ctx context.Context, req domain.CreateRiskCategoryRequest) (*domain.RiskCategory, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO risk_category (name, description, created_by, updated_by) VALUES (?, ?, ?, ?)",
		req.Name, nullableString(req.Description), req.CreatedBy, req.CreatedBy)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, &apierror.ConflictError{Msg: fmt.Sprintf("a risk category named %q already exists", req.Name)}
		}
		return nil, fmt.Errorf("risk_category.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetRiskCategoryByID(ctx, int(id))
}

func (r *riskCategoryRepo) UpdateRiskCategory(ctx context.Context, id int, req domain.UpdateRiskCategoryRequest) (*domain.RiskCategory, error) {
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
		"UPDATE risk_category SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		if isDuplicateKey(err) {
			return nil, &apierror.ConflictError{Msg: "a risk category with that name already exists"}
		}
		return nil, fmt.Errorf("risk_category.Update(%d): %w", id, err)
	}
	return r.GetRiskCategoryByID(ctx, id)
}

// DeleteRiskCategory removes a category outright — unlike risk_team/user,
// risk_category has no status column to soft-delete instead (see the schema
// comment on the table; it was never designed to be retired, only renamed).
//
// risk_category_reference.category_id has ON DELETE CASCADE back to this
// table, which means MySQL will not raise a constraint error for a category
// still tagged on risks — it will silently delete those risks' category tags
// along with the row. Checked explicitly here instead, before the DELETE
// runs, so a category that's actually in use is refused with a clear 409
// rather than quietly rewriting historical risk records.
func (r *riskCategoryRepo) DeleteRiskCategory(ctx context.Context, id int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("risk_category.Delete(%d) begin: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck

	// FOR UPDATE takes an exclusive lock on the category row for the rest of
	// this transaction. A concurrent INSERT into risk_category_reference for
	// this category_id must take a shared lock on this same row to satisfy
	// fk_rcat_category, so it blocks until this transaction commits or rolls
	// back — closing the window where a reference could be added between the
	// in-use count below and the DELETE and then silently vanish under the
	// cascade.
	var exists int
	if err := tx.QueryRowContext(ctx,
		"SELECT id FROM risk_category WHERE id = ? FOR UPDATE", id).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &apierror.NotFoundError{Msg: fmt.Sprintf("risk category %d not found", id)}
		}
		return fmt.Errorf("risk_category.Delete(%d) lock: %w", id, err)
	}

	var inUse int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM risk_category_reference WHERE category_id = ? FOR UPDATE", id).Scan(&inUse); err != nil {
		return fmt.Errorf("risk_category.Delete(%d) in-use check: %w", id, err)
	}
	if inUse > 0 {
		return &apierror.ConflictError{
			Msg: fmt.Sprintf("risk category is used by %d risk(s) and cannot be deleted", inUse)}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM risk_category WHERE id = ?", id); err != nil {
		return fmt.Errorf("risk_category.Delete(%d): %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("risk_category.Delete(%d) commit: %w", id, err)
	}
	return nil
}

func scanRiskCategory(s scanner) (*domain.RiskCategory, error) {
	var c domain.RiskCategory
	var description sql.NullString
	if err := s.Scan(&c.ID, &c.Name, &description); err != nil {
		return nil, err
	}
	if description.Valid {
		c.Description = &description.String
	}
	return &c, nil
}

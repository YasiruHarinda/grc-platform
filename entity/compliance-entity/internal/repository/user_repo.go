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

// Package repository provides MySQL-backed persistence for the compliance entity service.
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

// UserRepository defines persistence operations for the user table.
type UserRepository interface {
	SearchUsers(ctx context.Context, req domain.SearchUsersRequest) ([]domain.User, int, error)
	GetUserByID(ctx context.Context, id int) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateUser(ctx context.Context, req domain.CreateUserRequest) (*domain.User, error)
	UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (*domain.User, error)
}

type userRepo struct{ db *sql.DB }

// NewUserRepository constructs a UserRepository backed by the given connection pool.
func NewUserRepository(db *sql.DB) UserRepository { return &userRepo{db: db} }

func (r *userRepo) SearchUsers(ctx context.Context, req domain.SearchUsersRequest) ([]domain.User, int, error) {
	args := []any{}
	where := "WHERE 1=1"

	if req.SearchQuery != "" {
		where += " AND (email LIKE ? OR display_name LIKE ?)"
		p := "%" + likeEscape(req.SearchQuery) + "%"
		args = append(args, p, p)
	}
	if req.StatusKey != "" {
		where += " AND status = ?"
		args = append(args, req.StatusKey)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `user` "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("user.Search count: %w", err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, email, display_name, user_type, audit_team_id, risk_team_id, status, created_at, updated_at "+
			"FROM `user` "+where+" ORDER BY display_name LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("user.Search query: %w", err)
	}
	defer rows.Close()

	users := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("user.Search scan: %w", err)
		}
		users = append(users, *u)
	}
	return users, total, rows.Err()
}

func (r *userRepo) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, email, display_name, user_type, audit_team_id, risk_team_id, status, created_at, updated_at FROM `user` WHERE id = ?", id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("user %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("user.GetByID(%d): %w", id, err)
	}
	return u, nil
}

func (r *userRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, email, display_name, user_type, audit_team_id, risk_team_id, status, created_at, updated_at FROM `user` WHERE email = ?", email)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("user with email %q not found", email)}
	}
	if err != nil {
		return nil, fmt.Errorf("user.GetByEmail(%q): %w", email, err)
	}
	return u, nil
}

func (r *userRepo) CreateUser(ctx context.Context, req domain.CreateUserRequest) (*domain.User, error) {
	status := req.Status
	if status == "" {
		status = "ACTIVE"
	}
	userType := req.UserType
	if userType == "" {
		userType = "INTERNAL"
	}
	// Upsert on uq_user_email: callers provisioning a user from an external
	// directory (e.g. an HR employee picked as a risk's Action Owner) can't know
	// whether the email is already known, so a duplicate refreshes the display
	// name instead of failing. Only display_name/updated_by are refreshed —
	// team assignments and status stay under explicit UpdateUser control.
	//
	// id = LAST_INSERT_ID(id) is required: without it LastInsertId() returns 0
	// when the duplicate-key branch fires, and the GetUserByID below would miss.
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO `user` (email, display_name, user_type, audit_team_id, risk_team_id, status, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE display_name = VALUES(display_name), updated_by = VALUES(updated_by), id = LAST_INSERT_ID(id)",
		req.Email, req.DisplayName, userType, nullableInt(req.AuditTeamID), nullableInt(req.RiskTeamID), status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("user.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.GetUserByID(ctx, int(id))
}

func (r *userRepo) UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (*domain.User, error) {
	sets := []string{}
	args := []any{}

	if req.DisplayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, *req.DisplayName)
	}
	if req.AuditTeamID != nil {
		sets = append(sets, "audit_team_id = ?")
		args = append(args, *req.AuditTeamID)
	}
	if req.RiskTeamID != nil {
		sets = append(sets, "risk_team_id = ?")
		args = append(args, *req.RiskTeamID)
	}
	if req.UserType != nil {
		sets = append(sets, "user_type = ?")
		args = append(args, *req.UserType)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)
	args = append(args, id)

	if _, err := r.db.ExecContext(ctx,
		"UPDATE `user` SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("user.Update(%d): %w", id, err)
	}
	return r.GetUserByID(ctx, id)
}

func scanUser(s scanner) (*domain.User, error) {
	var u domain.User
	var auditTeamID, riskTeamID sql.NullInt64
	if err := s.Scan(&u.ID, &u.Email, &u.DisplayName, &u.UserType, &auditTeamID, &riskTeamID, &u.Status, &u.CreatedOn, &u.UpdatedOn); err != nil {
		return nil, err
	}
	if auditTeamID.Valid {
		v := int(auditTeamID.Int64)
		u.AuditTeamID = &v
	}
	if riskTeamID.Valid {
		v := int(riskTeamID.Int64)
		u.RiskTeamID = &v
	}
	return &u, nil
}

// nullableInt converts *int to sql.NullInt64 for optional FK columns.
func nullableInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

// ValidUserStatus reports whether s is a recognised user status value.
func ValidUserStatus(s string) bool {
	return s == "" || map[string]bool{"ACTIVE": true, "INACTIVE": true, "REMOVED": true}[strings.ToUpper(s)]
}

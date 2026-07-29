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
		"SELECT id, email, display_name, user_type, risk_team_id, status, created_at, updated_at "+
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	ids := make([]int, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	teamsByUser, err := r.loadAuditTeamIDsBatch(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range users {
		users[i].AuditTeamIDs = teamsByUser[users[i].ID]
	}
	return users, total, nil
}

func (r *userRepo) GetUserByID(ctx context.Context, id int) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, email, display_name, user_type, risk_team_id, status, created_at, updated_at FROM `user` WHERE id = ?", id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("user %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("user.GetByID(%d): %w", id, err)
	}
	if u.AuditTeamIDs, err = r.loadAuditTeamIDs(ctx, u.ID); err != nil {
		return nil, err
	}
	return u, nil
}

func (r *userRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT id, email, display_name, user_type, risk_team_id, status, created_at, updated_at FROM `user` WHERE email = ?", email)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("user with email %q not found", email)}
	}
	if err != nil {
		return nil, fmt.Errorf("user.GetByEmail(%q): %w", email, err)
	}
	if u.AuditTeamIDs, err = r.loadAuditTeamIDs(ctx, u.ID); err != nil {
		return nil, err
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("user.Create begin: %w", err)
	}
	defer tx.Rollback()

	// Upsert on uq_user_email: callers provisioning a user from an external
	// directory (e.g. an HR employee picked as a risk's Action Owner) can't know
	// whether the email is already known, so a duplicate refreshes the display
	// name instead of failing. Only display_name/updated_by are refreshed —
	// team assignments and status stay under explicit UpdateUser control, so
	// audit team rows below are only written when this was a genuine insert.
	//
	// id = LAST_INSERT_ID(id) is required: without it LastInsertId() returns 0
	// when the duplicate-key branch fires, and the GetUserByID below would miss.
	res, err := tx.ExecContext(ctx,
		"INSERT INTO `user` (email, display_name, user_type, risk_team_id, status, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE display_name = VALUES(display_name), updated_by = VALUES(updated_by), id = LAST_INSERT_ID(id)",
		req.Email, req.DisplayName, userType, nullableInt(req.RiskTeamID), status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("user.Create: %w", err)
	}
	id64, _ := res.LastInsertId()
	id := int(id64)

	// RowsAffected is 1 for a fresh insert under ON DUPLICATE KEY UPDATE, and
	// 0 or 2 when the duplicate-key branch fired instead — only a fresh insert
	// gets its requested team memberships written.
	if n, _ := res.RowsAffected(); n == 1 {
		for _, teamID := range req.AuditTeamIDs {
			if _, err = tx.ExecContext(ctx,
				"INSERT INTO user_audit_team (user_id, audit_team_id, created_by, updated_by) VALUES (?, ?, ?, ?)",
				id, teamID, req.CreatedBy, req.CreatedBy); err != nil {
				if isFKViolation(err) {
					return nil, &apierror.ValidationError{Msg: fmt.Sprintf("audit team %d not found", teamID)}
				}
				return nil, fmt.Errorf("user.Create audit team %d: %w", teamID, err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("user.Create commit: %w", err)
	}
	return r.GetUserByID(ctx, id)
}

func (r *userRepo) UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (*domain.User, error) {
	sets := []string{}
	args := []any{}

	if req.DisplayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, *req.DisplayName)
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("user.Update(%d) begin: %w", id, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		"UPDATE `user` SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil { // #nosec G202
		return nil, fmt.Errorf("user.Update(%d): %w", id, err)
	}

	// AuditTeamIDs nil means "not touching them"; a non-nil slice is the
	// complete desired set, so replace wholesale — same convention as
	// UpdateRiskRequest.ComplianceReferenceIDs.
	if req.AuditTeamIDs != nil {
		if _, err = tx.ExecContext(ctx,
			"DELETE FROM user_audit_team WHERE user_id = ?", id); err != nil {
			return nil, fmt.Errorf("user.Update(%d) clear audit teams: %w", id, err)
		}
		for _, teamID := range req.AuditTeamIDs {
			if _, err = tx.ExecContext(ctx,
				"INSERT INTO user_audit_team (user_id, audit_team_id, created_by, updated_by) VALUES (?, ?, ?, ?)",
				id, teamID, req.UpdatedBy, req.UpdatedBy); err != nil {
				if isFKViolation(err) {
					return nil, &apierror.ValidationError{Msg: fmt.Sprintf("audit team %d not found", teamID)}
				}
				return nil, fmt.Errorf("user.Update(%d) audit team %d: %w", id, teamID, err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("user.Update(%d) commit: %w", id, err)
	}
	return r.GetUserByID(ctx, id)
}

// loadAuditTeamIDs returns the audit team ids a single user belongs to.
func (r *userRepo) loadAuditTeamIDs(ctx context.Context, userID int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT audit_team_id FROM user_audit_team WHERE user_id = ? ORDER BY audit_team_id", userID)
	if err != nil {
		return nil, fmt.Errorf("user_audit_team.load(%d): %w", userID, err)
	}
	defer rows.Close()

	ids := []int{}
	for rows.Next() {
		var teamID int
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("user_audit_team.load(%d) scan: %w", userID, err)
		}
		ids = append(ids, teamID)
	}
	return ids, rows.Err()
}

// loadAuditTeamIDsBatch returns each user's audit team ids in one query, so a
// page of search results costs one extra round trip rather than one per row.
func (r *userRepo) loadAuditTeamIDsBatch(ctx context.Context, userIDs []int) (map[int][]int, error) {
	out := map[int][]int{}
	if len(userIDs) == 0 {
		return out, nil
	}

	ph := strings.Repeat("?,", len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT user_id, audit_team_id FROM user_audit_team WHERE user_id IN ("+ph[:len(ph)-1]+") ORDER BY user_id, audit_team_id",
		args...)
	if err != nil {
		return nil, fmt.Errorf("user_audit_team.loadBatch: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, teamID int
		if err := rows.Scan(&userID, &teamID); err != nil {
			return nil, fmt.Errorf("user_audit_team.loadBatch scan: %w", err)
		}
		out[userID] = append(out[userID], teamID)
	}
	return out, rows.Err()
}

func scanUser(s scanner) (*domain.User, error) {
	var u domain.User
	var riskTeamID sql.NullInt64
	if err := s.Scan(&u.ID, &u.Email, &u.DisplayName, &u.UserType, &riskTeamID, &u.Status, &u.CreatedOn, &u.UpdatedOn); err != nil {
		return nil, err
	}
	if riskTeamID.Valid {
		v := int(riskTeamID.Int64)
		u.RiskTeamID = &v
	}
	u.AuditTeamIDs = []int{}
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

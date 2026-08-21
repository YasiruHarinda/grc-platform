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
	GetUserByUUID(ctx context.Context, uuid string) (*domain.User, error)
	CreateUser(ctx context.Context, req domain.CreateUserRequest) (*domain.User, error)
	UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (*domain.User, error)
}

type userRepo struct{ db *sql.DB }

// NewUserRepository constructs a UserRepository backed by the given connection pool.
func NewUserRepository(db *sql.DB) UserRepository { return &userRepo{db: db} }

// userColumns is the select list every single-user read shares, so a new column
// cannot be added to one path and forgotten in another — scanUser expects
// exactly this order.
const userColumns = "id, uuid, user_type, status, created_at, updated_at"

func (r *userRepo) SearchUsers(ctx context.Context, req domain.SearchUsersRequest) ([]domain.User, int, error) {
	args := []any{}
	where := "WHERE 1=1"

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
		"SELECT "+userColumns+" FROM `user` "+where+" ORDER BY id LIMIT ? OFFSET ?",
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
	if err := r.attachRiskTeams(ctx, users); err != nil {
		return nil, 0, fmt.Errorf("user.Search risk teams: %w", err)
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
	row := r.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM `user` WHERE id = ?", id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("user %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("user.GetByID(%d): %w", id, err)
	}
	if u.RiskTeamIDs, err = r.loadRiskTeamIDs(ctx, u.ID); err != nil {
		return nil, fmt.Errorf("user.GetByID(%d) risk teams: %w", id, err)
	}
	if u.AuditTeamIDs, err = r.loadAuditTeamIDs(ctx, u.ID); err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByUUID resolves a user by their Asgardeo id — the identity a token
// actually carries, and so the lookup the authenticated request path uses.
//
// An empty uuid is rejected rather than queried, even now that the column is
// NOT NULL: `WHERE uuid = ”` would simply match nothing, but refusing it here
// keeps a caller that forgot to resolve a uuid from silently getting a 404
// instead of an obviously-wrong request, and being handed an arbitrary user.
func (r *userRepo) GetUserByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	if strings.TrimSpace(uuid) == "" {
		return nil, &apierror.NotFoundError{Msg: "user with empty uuid not found"}
	}
	row := r.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM `user` WHERE uuid = ?", uuid)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("user with uuid %q not found", uuid)}
	}
	if err != nil {
		return nil, fmt.Errorf("user.GetByUUID(%q): %w", uuid, err)
	}
	if u.RiskTeamIDs, err = r.loadRiskTeamIDs(ctx, u.ID); err != nil {
		return nil, fmt.Errorf("user.GetByUUID(%q) risk teams: %w", uuid, err)
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

	uuid := strings.TrimSpace(req.UUID)
	if uuid == "" {
		return nil, &apierror.ValidationError{Msg: "uuid is required"}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("user.Create begin: %w", err)
	}
	defer tx.Rollback()

	// Upsert on uq_user_uuid, now the table's only unique key (uq_user_email
	// is gone along with the column): a caller provisioning a user resolves
	// their uuid first (e.g. against the identity directory) and this insert
	// refreshes the existing row rather than failing if one already exists
	// for it. Only updated_by is refreshed on a duplicate — team assignments
	// and status stay under explicit UpdateUser control, so audit team rows
	// below are only written when this was a genuine insert.
	//
	// id = LAST_INSERT_ID(id) is required: without it LastInsertId() returns 0
	// when the duplicate-key branch fires, and the GetUserByID below would miss.
	res, err := tx.ExecContext(ctx,
		"INSERT INTO `user` (uuid, user_type, status, created_by, updated_by) "+
			"VALUES (?, ?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE "+
			"updated_by = VALUES(updated_by), id = LAST_INSERT_ID(id)",
		uuid, userType, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("user.Create: %w", err)
	}
	id64, _ := res.LastInsertId()
	id := int(id64)

	// RowsAffected is 1 for a fresh insert under ON DUPLICATE KEY UPDATE, and
	// 0 or 2 when the duplicate-key branch fired instead — only a fresh insert
	// gets its requested audit team memberships written. Risk team membership
	// is never written here — see the CreateUserRequest comment.
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

	// A nil slice means "not touching it"; a non-nil slice (even empty) is the
	// complete desired set, so replace wholesale — same convention as
	// UpdateRiskRequest.ComplianceReferenceIDs. Runs inside the same
	// transaction as the `user` UPDATE above, so a failure partway through
	// can't leave membership in a state the caller never asked for. Risk team
	// membership is never written here — see the UpdateUserRequest comment;
	// user_risk_team is read-only now, superseded by user_role_grant.
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
		"SELECT audit_team_id FROM user_audit_team WHERE user_id = ? AND is_active = TRUE ORDER BY audit_team_id", userID)
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

// loadRiskTeamIDs returns a single user's risk-team memberships, ordered for
// stable output.
func (r *userRepo) loadRiskTeamIDs(ctx context.Context, userID int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT risk_team_id FROM user_risk_team WHERE user_id = ? ORDER BY risk_team_id", userID)
	if err != nil {
		return nil, fmt.Errorf("loadRiskTeamIDs(%d): %w", userID, err)
	}
	defer rows.Close()

	ids := []int{}
	for rows.Next() {
		var teamID int
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("loadRiskTeamIDs(%d) scan: %w", userID, err)
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
		"SELECT user_id, audit_team_id FROM user_audit_team WHERE user_id IN ("+ph[:len(ph)-1]+") AND is_active = TRUE ORDER BY user_id, audit_team_id",
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

// attachRiskTeams batches the user_risk_team lookup for a list of users
// (one IN-list query instead of one query per user) and populates each
// user's RiskTeamIDs in place.
func (r *userRepo) attachRiskTeams(ctx context.Context, users []domain.User) error {
	if len(users) == 0 {
		return nil
	}
	idx := make(map[int]int, len(users))
	placeholders := make([]string, len(users))
	args := make([]any, len(users))
	for i := range users {
		idx[users[i].ID] = i
		placeholders[i] = "?"
		args[i] = users[i].ID
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT user_id, risk_team_id FROM user_risk_team WHERE user_id IN ("+strings.Join(placeholders, ",")+")",
		args...)
	if err != nil {
		return fmt.Errorf("attachRiskTeams: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, teamID int
		if err := rows.Scan(&userID, &teamID); err != nil {
			return fmt.Errorf("attachRiskTeams scan: %w", err)
		}
		i := idx[userID]
		users[i].RiskTeamIDs = append(users[i].RiskTeamIDs, teamID)
	}
	return rows.Err()
}

func scanUser(s scanner) (*domain.User, error) {
	var u domain.User
	// Both team-membership slices are filled in by the caller from their
	// junction tables; initialise them so a user with no memberships serialises
	// as [] rather than null.
	u.AuditTeamIDs = []int{}
	u.RiskTeamIDs = []int{}
	// uuid is scanned nullable even though the column is meant to be NOT
	// NULL: a row created before its owner ever resolved through Asgardeo
	// (or a database whose NOT NULL migration hasn't landed yet) can still
	// carry a NULL here, and failing every other row's read over one
	// unresolved user is worse than showing that one user as identity-less.
	var uuid sql.NullString
	if err := s.Scan(&u.ID, &uuid, &u.UserType, &u.Status,
		&u.CreatedOn, &u.UpdatedOn); err != nil {
		return nil, err
	}
	u.UUID = uuid.String
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

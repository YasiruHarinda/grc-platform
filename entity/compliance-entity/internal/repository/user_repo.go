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
const userColumns = "id, uuid, email, display_name, user_type, status, created_at, updated_at"

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
		"SELECT "+userColumns+" FROM `user` "+where+" ORDER BY display_name LIMIT ? OFFSET ?",
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

func (r *userRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM `user` WHERE email = ?", email)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("user with email %q not found", email)}
	}
	if err != nil {
		return nil, fmt.Errorf("user.GetByEmail(%q): %w", email, err)
	}
	if u.RiskTeamIDs, err = r.loadRiskTeamIDs(ctx, u.ID); err != nil {
		return nil, fmt.Errorf("user.GetByEmail(%q) risk teams: %w", email, err)
	}
	if u.AuditTeamIDs, err = r.loadAuditTeamIDs(ctx, u.ID); err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByUUID resolves a user by their Asgardeo id — the identity a token
// actually carries, and so the lookup the authenticated request path uses.
//
// An empty uuid is rejected rather than queried. The column is nullable during
// the identity migration, so `WHERE uuid = ”` would match nothing while
// `WHERE uuid IS NULL` would match every un-backfilled row at once; treating a
// missing identity as NotFound keeps a caller that forgot to resolve one from
// being handed an arbitrary user.
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("user.Create begin: %w", err)
	}
	defer tx.Rollback()

	// Upsert on uq_user_email: callers provisioning a user from an external
	// directory (e.g. an HR employee picked as a risk's Action Owner) can't know
	// whether the email is already known, so a duplicate refreshes the row
	// instead of failing. Only updated_by/uuid are refreshed on a duplicate —
	// team assignments and status stay under explicit UpdateUser control, so
	// audit team rows below are only written when this was a genuine insert.
	//
	// display_name is NOT refreshed on a duplicate, deliberately: nothing
	// upstream stores a name anymore (callers send "" — see
	// user/handler/resolve.go), and VALUES(display_name) here would silently
	// blank out an existing row's stored name on every re-provision of the
	// same person. It is still written on a genuine INSERT, only to satisfy
	// the column's NOT NULL constraint — empty string, never read back by
	// anything, until the column itself is dropped.
	//
	// id = LAST_INSERT_ID(id) is required: without it LastInsertId() returns 0
	// when the duplicate-key branch fires, and the GetUserByID below would miss.
	//
	// uuid = COALESCE(VALUES(uuid), uuid), NOT VALUES(uuid): this upsert is how
	// an existing row that predates the identity migration acquires its uuid, so
	// the incoming value has to be able to fill a gap. But a caller who did not
	// resolve one sends NULL, and plain VALUES(uuid) would then erase a uuid the
	// row already had — silently unmaking the migration one provision at a time.
	// COALESCE fills only when the row has nothing.
	res, err := tx.ExecContext(ctx,
		"INSERT INTO `user` (uuid, email, display_name, user_type, status, created_by, updated_by) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE "+
			"updated_by = VALUES(updated_by), uuid = COALESCE(VALUES(uuid), uuid), id = LAST_INSERT_ID(id)",
		nullableUUID(req.UUID), nullableEmail(req.Email), req.DisplayName, userType, status, req.CreatedBy, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("user.Create: %w", err)
	}
	id64, _ := res.LastInsertId()
	id := int(id64)

	// Confirm the row we touched is the one identified by req.Email.
	//
	// There are two unique keys now, and ON DUPLICATE KEY UPDATE resolves a
	// conflict on EITHER of them — it does not raise 1062 for the second key.
	// So a request carrying someone else's uuid with a new email does not fail:
	// it matches uq_user_uuid and rewrites THAT person's row instead, returning
	// a success and an id belonging to a different human. A caller who then used
	// the id — to set a risk's Action Owner, say — would attach the wrong person.
	//
	// Verified empirically: posting {uuid: <A's uuid>, email: <B's email>}
	// renamed A rather than erroring.
	//
	// Reading the email back inside the transaction and comparing is what makes
	// that unrepresentable: a mismatch means the uuid belongs to someone else, so
	// the deferred Rollback undoes the write. EqualFold because uq_user_email is
	// case-insensitive (utf8mb4_unicode_ci), so the stored spelling may differ
	// from the requested one without being a different row.
	//
	// Skipped entirely when req.Email is empty (the admin console's uuid-only
	// path): an empty email makes no claim about the row's real email to
	// verify, and the row this upsert touched may well already have a real one
	// from an earlier, email-keyed provision (e.g. via the Action Owner
	// resolve flow) — that's a legitimate re-provision-by-uuid, not a hijack.
	if req.Email != "" {
		var storedEmail sql.NullString
		if err = tx.QueryRowContext(ctx, "SELECT email FROM `user` WHERE id = ?", id).Scan(&storedEmail); err != nil {
			return nil, fmt.Errorf("user.Create readback: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(storedEmail.String), strings.TrimSpace(req.Email)) {
			return nil, &apierror.ValidationError{
				Msg: fmt.Sprintf("uuid %q is already assigned to a different user", req.UUID),
			}
		}
	}

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

	if req.DisplayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, *req.DisplayName)
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
	// uuid is nullable while the identity migration is in progress — rows
	// predating it, or whose email matched no Asgardeo account, have none.
	// email is nullable too, for uuid-only rows provisioned via the Admin
	// Console (see nullableEmail). Both flattened to "" so callers test one
	// thing (empty) rather than two.
	var uuid, email sql.NullString
	if err := s.Scan(&u.ID, &uuid, &email, &u.DisplayName, &u.UserType, &u.Status,
		&u.CreatedOn, &u.UpdatedOn); err != nil {
		return nil, err
	}
	u.UUID = uuid.String
	u.Email = email.String
	return &u, nil
}

// nullableUUID maps an absent uuid to SQL NULL rather than the empty string.
//
// This is not cosmetic. uq_user_uuid permits any number of NULLs but only one
// ”, so storing "" would let the first user without a uuid succeed and make
// every subsequent one a duplicate-key failure.
func nullableUUID(uuid string) any {
	if strings.TrimSpace(uuid) == "" {
		return nil
	}
	return uuid
}

// nullableEmail maps an absent email to SQL NULL rather than the empty
// string — same reasoning as nullableUUID. uq_user_email permits any number
// of NULLs but only one "", so storing "" would let the first uuid-only
// (admin console) provision succeed and make every subsequent one a
// duplicate-key failure.
func nullableEmail(email string) any {
	if strings.TrimSpace(email) == "" {
		return nil
	}
	return email
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

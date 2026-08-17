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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// GrantRepository reads and writes user_role_grant — the record of which role a
// user holds in which scope.
type GrantRepository interface {
	GrantsForUserID(ctx context.Context, userID int) ([]domain.UserGrant, error)
	GrantsForUserEmail(ctx context.Context, email string) (int, []domain.UserGrant, error)
	CreateGrant(ctx context.Context, userID int, req domain.CreateUserGrantRequest) (*domain.UserGrant, error)
	RevokeGrant(ctx context.Context, userID, grantID int) error
	GetRoleByID(ctx context.Context, roleID int) (*domain.Role, error)
	ListRoles(ctx context.Context) ([]domain.Role, error)
	TeamExists(ctx context.Context, scopeType string, scopeID int) (bool, error)
	RoleCarriesPrivilege(ctx context.Context, roleID int, privilegeName string) (bool, error)
}

type grantRepo struct{ db *sql.DB }

// NewGrantRepository constructs a GrantRepository backed by the given pool.
func NewGrantRepository(db *sql.DB) GrantRepository { return &grantRepo{db: db} }

// grantSelect is shared by both read paths so they cannot drift apart. The
// LEFT JOINs resolve a scope's display name without the caller needing the team
// lists; each is guarded on scope_type so a RISK_TEAM id can never accidentally
// match an audit team with the same numeric id (scope_id has no FK — it points
// at two different tables — so nothing else prevents that).
//
// A team-scoped grant is excluded once its team is no longer ACTIVE.
// Deactivating a register/team (PATCH /risk/teams/{id} et al.) is a real,
// reachable admin action; without this, a grant scoped to a retired team would
// keep conferring full access to it forever — nothing else in the
// authorisation path re-checks team status once a grant is resolved.
//
// This has to live in the WHERE clause, not the LEFT JOINs' own ON conditions:
// a LEFT JOIN with a failing ON predicate still returns the grant row, just
// with the joined columns NULLed — it does not exclude the row. GLOBAL grants
// are exempted explicitly, since they match neither join at all (scope_id is
// the 0 sentinel) and must not be swept up by a team-status condition that
// does not apply to them.
const grantSelect = `
	SELECT g.id, g.role_id, r.role_name, r.module, COALESCE(r.scope_basis,'') AS scope_basis,
	       g.scope_type, g.scope_id,
	       COALESCE(rt.name, at.name, '') AS scope_name,
	       COALESCE(rt.team_type, IF(at.id IS NULL, '', 'AUDIT_TEAM')) AS scope_team_type,
	       g.created_at, COALESCE(g.created_by, '')
	FROM   user_role_grant g
	JOIN   ` + "`role`" + ` r ON r.id = g.role_id
	LEFT JOIN risk_team  rt ON g.scope_type = 'RISK_TEAM'  AND rt.id = g.scope_id
	LEFT JOIN audit_team at ON g.scope_type = 'AUDIT_TEAM' AND at.id = g.scope_id
	WHERE  g.status = 'ACTIVE'
	  AND  r.status = 'ACTIVE'
	  AND  (g.scope_type = 'GLOBAL'
	        OR (g.scope_type = 'RISK_TEAM'  AND rt.status = 'ACTIVE')
	        OR (g.scope_type = 'AUDIT_TEAM' AND at.status = 'ACTIVE'))`

// scanGrants materialises rows produced by grantSelect.
func scanGrants(rows *sql.Rows) ([]domain.UserGrant, error) {
	out := []domain.UserGrant{}
	for rows.Next() {
		var g domain.UserGrant
		if err := rows.Scan(&g.ID, &g.RoleID, &g.RoleName, &g.Module, &g.ScopeBasis,
			&g.ScopeType, &g.ScopeID, &g.ScopeName, &g.ScopeTeamType,
			&g.CreatedOn, &g.CreatedBy); err != nil {
			return nil, fmt.Errorf("grant scan: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GrantsForUserID returns every active grant held by a user.
//
// A grant is active only when both the join row and its role are active, so
// deactivating either revokes it — the same rule role_privilege follows.
func (r *grantRepo) GrantsForUserID(ctx context.Context, userID int) ([]domain.UserGrant, error) {
	rows, err := r.db.QueryContext(ctx,
		grantSelect+" AND g.user_id = ? ORDER BY r.role_name, g.scope_type, g.scope_id", userID)
	if err != nil {
		return nil, fmt.Errorf("grant.GrantsForUserID: %w", err)
	}
	defer rows.Close()
	return scanGrants(rows)
}

// GrantsForUserEmail resolves a user by email and returns their grants in one
// round trip, returning the resolved user id alongside.
//
// This is the hot path: the GRC backend calls it on every authenticated request
// to build the caller's scoped privilege set. Splitting it into "look up user,
// then look up grants" would double the per-request cost of every API call in
// the platform.
//
// A missing user is NotFound rather than an empty grant list, so the caller can
// distinguish "this person has no roles" from "this person does not exist here"
// — the two need different handling, and conflating them would silently treat
// an unprovisioned account as a legitimately unprivileged one.
func (r *grantRepo) GrantsForUserEmail(ctx context.Context, email string) (int, []domain.UserGrant, error) {
	var userID int
	err := r.db.QueryRowContext(ctx,
		"SELECT id FROM `user` WHERE email = ? AND status = 'ACTIVE'", email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, &apierror.NotFoundError{Msg: "user not found: " + email}
	}
	if err != nil {
		return 0, nil, fmt.Errorf("grant.GrantsForUserEmail lookup: %w", err)
	}

	grants, err := r.GrantsForUserID(ctx, userID)
	if err != nil {
		return 0, nil, err
	}
	return userID, grants, nil
}

// CreateGrant inserts a grant and returns it as stored.
//
// Re-granting something already held is not an error: the INSERT reactivates a
// previously revoked row rather than failing on uq_grant. Revocation sets
// status = 'INACTIVE' rather than deleting, so the row survives as a record
// that the grant once existed, and re-granting must be able to reuse it.
func (r *grantRepo) CreateGrant(ctx context.Context, userID int, req domain.CreateUserGrantRequest) (*domain.UserGrant, error) {
	// The insert result is deliberately discarded: LastInsertId is 0 on the
	// ON DUPLICATE KEY path when nothing changed, so the grant is read back by
	// its natural key below rather than trusted from here.
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_role_grant (user_id, role_id, scope_type, scope_id, status, created_by)
		VALUES (?, ?, ?, ?, 'ACTIVE', ?)
		ON DUPLICATE KEY UPDATE
			status     = 'ACTIVE',
			updated_by = VALUES(created_by)`,
		userID, req.RoleID, req.ScopeType, req.ScopeID, req.CreatedBy)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.ValidationError{Msg: "unknown user or role"}
		}
		return nil, fmt.Errorf("grant.CreateGrant: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		grantSelect+" AND g.user_id = ? AND g.role_id = ? AND g.scope_type = ? AND g.scope_id = ?",
		userID, req.RoleID, req.ScopeType, req.ScopeID)
	if err != nil {
		return nil, fmt.Errorf("grant.CreateGrant readback: %w", err)
	}
	defer rows.Close()
	grants, err := scanGrants(rows)
	if err != nil {
		return nil, err
	}
	if len(grants) == 0 {
		return nil, fmt.Errorf("grant.CreateGrant: row not found after insert")
	}
	return &grants[0], nil
}

// RevokeGrant deactivates a grant. The row is kept, not deleted: who held what,
// and when it was taken away, is exactly the history an authorisation change
// needs to leave behind.
//
// Scoped by user_id as well as grant id so a mismatched pair cannot revoke
// another user's grant by guessing an id.
func (r *grantRepo) RevokeGrant(ctx context.Context, userID, grantID int) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE user_role_grant SET status = 'INACTIVE' WHERE id = ? AND user_id = ?",
		grantID, userID)
	if err != nil {
		return fmt.Errorf("grant.RevokeGrant: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("grant.RevokeGrant rows: %w", err)
	}
	if n == 0 {
		return &apierror.NotFoundError{Msg: "grant not found"}
	}
	return nil
}

// GetRoleByID returns an active role, or NotFound.
func (r *grantRepo) GetRoleByID(ctx context.Context, roleID int) (*domain.Role, error) {
	var role domain.Role
	err := r.db.QueryRowContext(ctx,
		"SELECT id, role_name, COALESCE(description,''), module, COALESCE(scope_basis,''), status "+
			"FROM `role` WHERE id = ?",
		roleID).Scan(&role.ID, &role.RoleName, &role.Description, &role.Module, &role.ScopeBasis, &role.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: "role not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("grant.GetRoleByID: %w", err)
	}
	return &role, nil
}

// ListRoles returns every active role, for populating a grant editor.
func (r *grantRepo) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, role_name, COALESCE(description,''), module, COALESCE(scope_basis,''), status "+
			"FROM `role` WHERE status = 'ACTIVE' ORDER BY module, role_name")
	if err != nil {
		return nil, fmt.Errorf("grant.ListRoles: %w", err)
	}
	defer rows.Close()

	out := []domain.Role{}
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(&role.ID, &role.RoleName, &role.Description, &role.Module,
			&role.ScopeBasis, &role.Status); err != nil {
			return nil, fmt.Errorf("grant.ListRoles scan: %w", err)
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

// TeamExists reports whether an active team of the given scope kind exists.
//
// scope_id carries no foreign key — it references risk_team or audit_team
// depending on scope_type, which no single FK can express — so this check is
// what stands in for one. Without it a grant could name a team that does not
// exist, and while that fails closed at read time, it would sit in the table
// looking like real access.
func (r *grantRepo) TeamExists(ctx context.Context, scopeType string, scopeID int) (bool, error) {
	var query string
	switch scopeType {
	case domain.ScopeRiskTeam:
		query = "SELECT 1 FROM risk_team WHERE id = ? AND status = 'ACTIVE'"
	case domain.ScopeAuditTeam:
		query = "SELECT 1 FROM audit_team WHERE id = ? AND status = 'ACTIVE'"
	default:
		return false, fmt.Errorf("grant.TeamExists: unsupported scope type %q", scopeType)
	}
	var one int
	err := r.db.QueryRowContext(ctx, query, scopeID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("grant.TeamExists: %w", err)
	}
	return true, nil
}

// RoleCarriesPrivilege reports whether a role actively grants a named privilege.
// Used to enforce that grant-management is never handed out scoped.
func (r *grantRepo) RoleCarriesPrivilege(ctx context.Context, roleID int, privilegeName string) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1
		FROM   role_privilege rp
		JOIN   privilege p ON p.id = rp.privilege_id
		WHERE  rp.role_id = ? AND p.privilege_name = ?
		  AND  rp.is_active = TRUE AND p.status = 'ACTIVE'`,
		roleID, privilegeName).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("grant.RoleCarriesPrivilege: %w", err)
	}
	return true, nil
}

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

// GrantRepository reads and writes user_role_grant — the record of which role a
// user holds in which scope.
type GrantRepository interface {
	GrantsForUserID(ctx context.Context, userID int) ([]domain.UserGrant, error)
	GrantsForUserIDs(ctx context.Context, userIDs []int) (map[int][]domain.UserGrant, error)
	// GrantsForUserUUID also returns user_type, which the GRC backend's auth
	// middleware cross-checks against its own caller classification.
	GrantsForUserUUID(ctx context.Context, uuid string) (int, string, []domain.UserGrant, error)
	CreateGrant(ctx context.Context, userID int, req domain.CreateUserGrantRequest) (*domain.UserGrant, error)
	RevokeGrant(ctx context.Context, userID, grantID int, revokedBy string) error
	GetRoleByID(ctx context.Context, roleID int) (*domain.Role, error)
	ListRoles(ctx context.Context) ([]domain.Role, error)
	TeamExists(ctx context.Context, scopeType string, scopeID int) (bool, error)
	RoleCarriesPrivilege(ctx context.Context, roleID int, privilegeName string) (bool, error)
	// RolesCarryingAnyPrivilege returns the role ids that carry at least one of
	// privilegeNames.
	RolesCarryingAnyPrivilege(ctx context.Context, privilegeNames []string) (map[int]bool, error)
	CandidatesForPrivilege(ctx context.Context, privilegeName string, teamIDs []int) ([]domain.GrantCandidate, error)
	// UserType returns the target user's user_type (INTERNAL/EXTERNAL), for
	// validateScope's assignable_user_type check.
	UserType(ctx context.Context, userID int) (string, error)
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
	SELECT g.user_id, g.id, g.role_id, r.role_name, r.module, COALESCE(r.scope_basis,'') AS scope_basis,
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

// scanGrants materialises rows produced by grantSelect. The leading user_id
// column is discarded here — every caller except GrantsForUserIDs already
// knows which user it asked for.
func scanGrants(rows *sql.Rows) ([]domain.UserGrant, error) {
	out := []domain.UserGrant{}
	for rows.Next() {
		var g domain.UserGrant
		var userID int
		if err := rows.Scan(&userID, &g.ID, &g.RoleID, &g.RoleName, &g.Module, &g.ScopeBasis,
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

// GrantsForUserIDs returns every active grant for each of the given users, in
// one query — the Admin Console's user list needs every row's grants at once,
// and a query per row would turn a page of N users into N+1 round trips.
// Callers whose userIDs is empty get an empty map without querying.
func (r *grantRepo) GrantsForUserIDs(ctx context.Context, userIDs []int) (map[int][]domain.UserGrant, error) {
	out := map[int][]domain.UserGrant{}
	if len(userIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := r.db.QueryContext(ctx,
		grantSelect+" AND g.user_id IN ("+strings.Join(placeholders, ",")+") ORDER BY g.user_id, r.role_name, g.scope_type, g.scope_id",
		args...)
	if err != nil {
		return nil, fmt.Errorf("grant.GrantsForUserIDs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		var g domain.UserGrant
		if err := rows.Scan(&userID, &g.ID, &g.RoleID, &g.RoleName, &g.Module, &g.ScopeBasis,
			&g.ScopeType, &g.ScopeID, &g.ScopeName, &g.ScopeTeamType,
			&g.CreatedOn, &g.CreatedBy); err != nil {
			return nil, fmt.Errorf("grant.GrantsForUserIDs scan: %w", err)
		}
		out[userID] = append(out[userID], g)
	}
	return out, rows.Err()
}

// GrantsForUserUUID resolves a user by their Asgardeo id and returns their
// grants, returning the resolved user id alongside — the identity a token
// actually carries, so no translation step stands between authenticating a
// caller and authorising them.
//
// This is the hot path: the GRC backend calls it on every authenticated
// request to build the caller's scoped privilege set, and consumes both halves
// of the result (see middleware.UserInfo.UserID). Exposing it as one endpoint
// rather than making callers chain GET /users/by-uuid + GET /grants/user/{id}
// is what keeps that to a single HTTP round trip — the cost worth saving here,
// since the network hop dominates.
//
// It does take two queries internally, deliberately. Folding them into one
// join would mean either forking grantSelect — which both read paths share
// precisely so they cannot drift apart — or LEFT JOINing from `user` and
// scanning nullable grant columns, because a user with no grants produces no
// grant rows and would otherwise be indistinguishable from one who does not
// exist. That distinction is load-bearing (see below), and a second indexed
// primary-key lookup on an already-open pooled connection is not what this
// path spends its time on.
//
// A missing user is NotFound rather than an empty grant list, so the caller can
// distinguish "this person has no roles" from "this person does not exist here"
// — the two need different handling, and conflating them would silently treat
// an unprovisioned account as a legitimately unprivileged one.
//
// An empty uuid is NotFound rather than a query — refusing it here keeps a
// blank string from ever becoming a way to resolve an arbitrary user's grants.
func (r *grantRepo) GrantsForUserUUID(ctx context.Context, uuid string) (int, string, []domain.UserGrant, error) {
	if strings.TrimSpace(uuid) == "" {
		return 0, "", nil, &apierror.NotFoundError{Msg: "user not found: empty uuid"}
	}
	var userID int
	var userType string
	// user_type rides along on the lookup already running here.
	err := r.db.QueryRowContext(ctx,
		"SELECT id, user_type FROM `user` WHERE uuid = ? AND status = 'ACTIVE'", uuid).Scan(&userID, &userType)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil, &apierror.NotFoundError{Msg: "user not found for uuid"}
	}
	if err != nil {
		return 0, "", nil, fmt.Errorf("grant.GrantsForUserUUID lookup: %w", err)
	}

	grants, err := r.GrantsForUserID(ctx, userID)
	if err != nil {
		return 0, "", nil, err
	}
	return userID, userType, grants, nil
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
// revokedBy lands in updated_by, so the row records who took the grant away as
// well as who gave it (created_by). Without it a revoked row still names only
// the grantor, which reads as if they performed both halves — and removing
// someone's access deserves the same audit trail as conferring it.
//
// Scoped by user_id as well as grant id so a mismatched pair cannot revoke
// another user's grant by guessing an id.
func (r *grantRepo) RevokeGrant(ctx context.Context, userID, grantID int, revokedBy string) error {
	res, err := r.db.ExecContext(ctx,
		"UPDATE user_role_grant SET status = 'INACTIVE', updated_by = ? WHERE id = ? AND user_id = ?",
		revokedBy, grantID, userID)
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
		"SELECT id, role_name, COALESCE(description,''), module, COALESCE(scope_basis,''), assignable_user_type, status "+
			"FROM `role` WHERE id = ?",
		roleID).Scan(&role.ID, &role.RoleName, &role.Description, &role.Module, &role.ScopeBasis,
		&role.AssignableUserType, &role.Status)
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
		"SELECT id, role_name, COALESCE(description,''), module, COALESCE(scope_basis,''), assignable_user_type, status "+
			"FROM `role` WHERE status = 'ACTIVE' ORDER BY module, role_name")
	if err != nil {
		return nil, fmt.Errorf("grant.ListRoles: %w", err)
	}
	defer rows.Close()

	out := []domain.Role{}
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(&role.ID, &role.RoleName, &role.Description, &role.Module,
			&role.ScopeBasis, &role.AssignableUserType, &role.Status); err != nil {
			return nil, fmt.Errorf("grant.ListRoles scan: %w", err)
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

// UserType returns the target user's user_type (INTERNAL/EXTERNAL), for
// validateScope's assignable_user_type check. NotFound mirrors CreateGrant's
// existing FK-violation handling — an unknown userID is a validation error,
// not a server error.
func (r *grantRepo) UserType(ctx context.Context, userID int) (string, error) {
	var userType string
	err := r.db.QueryRowContext(ctx, "SELECT user_type FROM `user` WHERE id = ?", userID).Scan(&userType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &apierror.NotFoundError{Msg: "user not found"}
	}
	if err != nil {
		return "", fmt.Errorf("grant.UserType: %w", err)
	}
	return userType, nil
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

// CandidatesForPrivilege returns every active user who holds privilegeName —
// GLOBAL, or scoped to one of teamIDs. Powers the Risk Hub's Owner /
// Management-Approver pickers: a candidate is exactly someone who would pass
// the register-scoped RequirePrivilegeIn check that role's approval action
// runs, so a picked candidate can never 403 on their first approval the way an
// Asgardeo-group-sourced candidate could.
//
// RISK_TEAM only. The audit module's notify.go callers (added in this PR)
// pass nil teamIDs deliberately, so today they only resolve GLOBAL grant
// holders — an AUDIT_TEAM-scoped admin or internal-comment viewer is not yet
// resolved as a notification recipient. Extending this with an AUDIT_TEAM
// branch, and threading a control's team through those callers, is tracked as
// follow-up work rather than done here.
func (r *grantRepo) CandidatesForPrivilege(ctx context.Context, privilegeName string, teamIDs []int) ([]domain.GrantCandidate, error) {
	args := []any{privilegeName}
	scopeCond := "g.scope_type = 'GLOBAL'"
	if len(teamIDs) > 0 {
		placeholders := make([]string, len(teamIDs))
		for i, id := range teamIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		scopeCond += " OR (g.scope_type = 'RISK_TEAM' AND rt.status = 'ACTIVE' AND g.scope_id IN (" +
			strings.Join(placeholders, ",") + "))"
	}
	// uuid only — the platform stores no name or email for anyone. The caller
	// resolves each candidate's current name from the identity directory.
	//
	// COALESCE, because uuid is nullable until every row is backfilled: a
	// candidate whose uuid is still NULL comes back as "" rather than requiring
	// every scan target here to be nullable. A caller that can't resolve ""
	// drops that candidate rather than offering someone with no name.
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT u.id, COALESCE(u.uuid, '')
		FROM   user_role_grant g
		JOIN   `+"`role`"+` r    ON r.id = g.role_id AND r.status = 'ACTIVE'
		JOIN   role_privilege rp ON rp.role_id = r.id AND rp.is_active = TRUE
		JOIN   privilege p       ON p.id = rp.privilege_id AND p.status = 'ACTIVE' AND p.privilege_name = ?
		JOIN   `+"`user`"+` u    ON u.id = g.user_id AND u.status = 'ACTIVE'
		LEFT JOIN risk_team rt   ON g.scope_type = 'RISK_TEAM' AND rt.id = g.scope_id
		WHERE  g.status = 'ACTIVE' AND (`+scopeCond+`)
		ORDER BY u.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("grant.CandidatesForPrivilege: %w", err)
	}
	defer rows.Close()

	out := []domain.GrantCandidate{}
	for rows.Next() {
		var c domain.GrantCandidate
		if err := rows.Scan(&c.ID, &c.UUID); err != nil {
			return nil, fmt.Errorf("grant.CandidatesForPrivilege scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
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

// RolesCarryingAnyPrivilege returns the role ids that carry at least one of
// privilegeNames.
func (r *grantRepo) RolesCarryingAnyPrivilege(ctx context.Context, privilegeNames []string) (map[int]bool, error) {
	out := map[int]bool{}
	if len(privilegeNames) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(privilegeNames))
	args := make([]any, len(privilegeNames))
	for i, name := range privilegeNames {
		placeholders[i] = "?"
		args[i] = name
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT rp.role_id
		FROM   role_privilege rp
		JOIN   privilege p ON p.id = rp.privilege_id
		WHERE  rp.is_active = TRUE AND p.status = 'ACTIVE'
		  AND  p.privilege_name IN (`+strings.Join(placeholders, ",")+`)`, // #nosec G202 -- concatenated part is only "?" placeholders (count-derived), values go through args
		args...)
	if err != nil {
		return nil, fmt.Errorf("grant.RolesCarryingAnyPrivilege: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var roleID int
		if err := rows.Scan(&roleID); err != nil {
			return nil, fmt.Errorf("grant.RolesCarryingAnyPrivilege scan: %w", err)
		}
		out[roleID] = true
	}
	return out, rows.Err()
}

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

package service

import (
	"context"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// manageUsersPrivilege gates User Management: provisioning users and granting
// or revoking roles. A role carrying it may only ever be granted GLOBAL — see
// validateGrant.
const manageUsersPrivilege = "MANAGE_USERS"

// globalOnlyPrivileges are privileges whose platform-side handlers check them
// with the unscoped HasPrivilege/RequirePrivilege, not HasPrivilegeIn, on the
// assumption that they are never granted team-scoped (see control.go,
// comment.go, audit.go, review.go and reminder.go in the platform backend). A
// role carrying any of these may only ever be granted GLOBAL, so that
// assumption holds instead of being a comment nobody enforces.
var globalOnlyPrivileges = []string{
	manageUsersPrivilege,
	"AUDIT_MANAGE_CONTROLS",
	"AUDIT_UPDATE_AUDIT",
}

// GrantService reads and writes role grants.
//
// Deliberately has no cached counterpart, unlike UserService. Grants are
// authorisation data on the revocation path: an admin who removes a grant must
// see it take effect on the caller's next request. A TTL cache here would mean
// "revoked within N minutes, on most replicas", which is not something that can
// be stated honestly in a security review.
type GrantService interface {
	GrantsForUserID(ctx context.Context, userID int) (domain.UserGrantsResponse, error)
	// GrantsForUserIDs batches GrantsForUserID for a page of users (the Admin
	// Console's user list), so embedding grants per row costs one query, not
	// one per row. Every id in userIDs is present in the result, even with an
	// empty (never nil) slice.
	GrantsForUserIDs(ctx context.Context, userIDs []int) (map[int][]domain.UserGrant, error)
	GrantsForUserUUID(ctx context.Context, uuid string) (domain.UserGrantsResponse, error)
	CreateGrant(ctx context.Context, userID int, req domain.CreateUserGrantRequest) (domain.UserGrant, error)
	RevokeGrant(ctx context.Context, userID, grantID int, revokedBy string) error
	ListRoles(ctx context.Context) (domain.ListRolesResponse, error)
	CandidatesForPrivilege(ctx context.Context, privilegeName string, teamIDs []int) (domain.GrantCandidatesResponse, error)
}

type grantService struct {
	repo repository.GrantRepository
}

// NewGrantService constructs a GrantService.
func NewGrantService(repo repository.GrantRepository) GrantService {
	return &grantService{repo: repo}
}

// scopesForModule lists the scope types a role of each module may be granted
// against. The role decides the module; the scope decides the breadth.
//
// SHARED roles are GLOBAL-only by construction: a role spanning both hubs has
// no single team table its scope could point at.
var scopesForModule = map[string][]string{
	domain.ModuleRisk:   {domain.ScopeGlobal, domain.ScopeRiskTeam},
	domain.ModuleAudit:  {domain.ScopeGlobal, domain.ScopeAuditTeam},
	domain.ModuleShared: {domain.ScopeGlobal},
}

func (s *grantService) GrantsForUserID(ctx context.Context, userID int) (domain.UserGrantsResponse, error) {
	if userID <= 0 {
		return domain.UserGrantsResponse{}, &apierror.ValidationError{Msg: "user id must be a positive integer"}
	}
	grants, err := s.repo.GrantsForUserID(ctx, userID)
	if err != nil {
		return domain.UserGrantsResponse{}, err
	}
	return domain.UserGrantsResponse{UserID: userID, Grants: orEmpty(grants)}, nil
}

func (s *grantService) GrantsForUserIDs(ctx context.Context, userIDs []int) (map[int][]domain.UserGrant, error) {
	byUser, err := s.repo.GrantsForUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int][]domain.UserGrant, len(userIDs))
	for _, id := range userIDs {
		out[id] = orEmpty(byUser[id])
	}
	return out, nil
}

func (s *grantService) GrantsForUserUUID(ctx context.Context, uuid string) (domain.UserGrantsResponse, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.UserGrantsResponse{}, &apierror.ValidationError{Msg: "uuid is required"}
	}
	userID, grants, err := s.repo.GrantsForUserUUID(ctx, uuid)
	if err != nil {
		return domain.UserGrantsResponse{}, err
	}
	return domain.UserGrantsResponse{UserID: userID, Grants: orEmpty(grants)}, nil
}

// orEmpty guarantees a slice serialises as [] rather than null. A caller must
// be able to tell "this user holds no roles" from a malformed response, since
// holding no roles is a legitimate state here.
func orEmpty(g []domain.UserGrant) []domain.UserGrant {
	if g == nil {
		return []domain.UserGrant{}
	}
	return g
}

// CreateGrant validates a grant against the role's module before storing it.
//
// The database cannot enforce most of this: scope_id has no foreign key
// (it references two different tables depending on scope_type), and nothing in
// SQL relates a role's module to the scope kind chosen for it. The CHECK
// constraint only catches the GLOBAL/scope_id sentinel mismatch. Everything
// else is enforced here, which makes this the single place a bad grant can be
// stopped before it looks like real access in the table.
func (s *grantService) CreateGrant(ctx context.Context, userID int, req domain.CreateUserGrantRequest) (domain.UserGrant, error) {
	if userID <= 0 {
		return domain.UserGrant{}, &apierror.ValidationError{Msg: "user id must be a positive integer"}
	}
	if req.RoleID <= 0 {
		return domain.UserGrant{}, &apierror.ValidationError{Msg: "roleId is required"}
	}

	req.ScopeType = strings.ToUpper(strings.TrimSpace(req.ScopeType))
	if req.ScopeType == "" {
		return domain.UserGrant{}, &apierror.ValidationError{
			Msg: "scopeType is required (GLOBAL, RISK_TEAM, or AUDIT_TEAM)"}
	}

	role, err := s.repo.GetRoleByID(ctx, req.RoleID)
	if err != nil {
		return domain.UserGrant{}, err
	}
	if role.Status != "ACTIVE" {
		return domain.UserGrant{}, &apierror.ValidationError{
			Msg: "role " + role.RoleName + " is inactive and cannot be granted"}
	}

	if err := s.validateUserType(ctx, userID, role); err != nil {
		return domain.UserGrant{}, err
	}

	if err := s.validateScope(ctx, role, &req); err != nil {
		return domain.UserGrant{}, err
	}

	grant, err := s.repo.CreateGrant(ctx, userID, req)
	if err != nil {
		return domain.UserGrant{}, err
	}
	return *grant, nil
}

// validateUserType enforces role.assignable_user_type: an INTERNAL-only role
// may never be granted to an EXTERNAL user. The other direction is
// deliberately open — an EXTERNAL-assignable role (currently only
// external-auditor) may be granted to an INTERNAL person too, since its
// authorisation is derived from control assignment (requireAssignedAuditor),
// not from the grantee's own identity type. This is the actual gate; a role
// picker that filters by this column client-side is a convenience on top,
// never a substitute for it.
func (s *grantService) validateUserType(ctx context.Context, userID int, role *domain.Role) error {
	if role.AssignableUserType == domain.UserTypeExternal {
		return nil
	}
	userType, err := s.repo.UserType(ctx, userID)
	if err != nil {
		return err
	}
	if userType != domain.UserTypeInternal {
		return &apierror.ValidationError{
			Msg: role.RoleName + " may only be granted to an INTERNAL user, and this user is " + userType}
	}
	return nil
}

// validateScope enforces the four rules the schema cannot, mutating req's
// ScopeID to the sentinel where required.
func (s *grantService) validateScope(ctx context.Context, role *domain.Role, req *domain.CreateUserGrantRequest) error {
	// 1. The scope kind must be one the role's module admits.
	allowed, ok := scopesForModule[role.Module]
	if !ok {
		return &apierror.ValidationError{Msg: "role has an unrecognised module: " + role.Module}
	}
	if !contains(allowed, req.ScopeType) {
		return &apierror.ValidationError{
			Msg: role.RoleName + " is a " + role.Module + " role and can only be granted " +
				strings.Join(allowed, " or ") + ", not " + req.ScopeType}
	}

	// 2. An EXTERNAL-assignable role is GLOBAL-only. Its effective scope is the
	// set of controls the person is assigned to (see requireAssignedAuditor in
	// the platform backend), so a team on it is inert data that nonetheless
	// reads as a real restriction wherever grants are displayed. GrantPicker
	// locks the scope picker for these roles; this is the enforcement, on the
	// same footing as validateUserType.
	if role.AssignableUserType == domain.UserTypeExternal && req.ScopeType != domain.ScopeGlobal {
		return &apierror.ValidationError{
			Msg: role.RoleName + " may only be granted GLOBAL: its scope comes from the controls " +
				"assigned to the person, not from a team"}
	}

	// 3. Some privileges are never handed out scoped — see globalOnlyPrivileges.
	// For MANAGE_USERS specifically: a register-scoped admin granting roles
	// would need the escalation rule that a grantor may not exceed their own
	// privileges in that scope — subtle, and silent when wrong — so it is
	// deliberately not permitted yet. Relaxing this later needs no schema change.
	if req.ScopeType != domain.ScopeGlobal {
		for _, priv := range globalOnlyPrivileges {
			carries, err := s.repo.RoleCarriesPrivilege(ctx, role.ID, priv)
			if err != nil {
				return err
			}
			if carries {
				return &apierror.ValidationError{
					Msg: role.RoleName + " carries " + priv +
						" and may only be granted GLOBAL"}
			}
		}
	}

	// 4. The scope must exist. GLOBAL is a wildcard and takes the sentinel;
	// anything else must name a live team, since scope_id has no FK to enforce it.
	if req.ScopeType == domain.ScopeGlobal {
		req.ScopeID = domain.GlobalScopeID
		return nil
	}
	if req.ScopeID <= 0 {
		return &apierror.ValidationError{Msg: "scopeId is required when scopeType is " + req.ScopeType}
	}
	exists, err := s.repo.TeamExists(ctx, req.ScopeType, req.ScopeID)
	if err != nil {
		return err
	}
	if !exists {
		return &apierror.ValidationError{Msg: "no active team found for the given scopeId"}
	}
	return nil
}

func (s *grantService) RevokeGrant(ctx context.Context, userID, grantID int, revokedBy string) error {
	if userID <= 0 {
		return &apierror.ValidationError{Msg: "user id must be a positive integer"}
	}
	if grantID <= 0 {
		return &apierror.ValidationError{Msg: "grant id must be a positive integer"}
	}
	revokedBy = strings.TrimSpace(revokedBy)
	if revokedBy == "" {
		return &apierror.ValidationError{Msg: "revokedBy is required"}
	}
	return s.repo.RevokeGrant(ctx, userID, grantID, revokedBy)
}

func (s *grantService) ListRoles(ctx context.Context) (domain.ListRolesResponse, error) {
	roles, err := s.repo.ListRoles(ctx)
	if err != nil {
		return domain.ListRolesResponse{}, err
	}
	if roles == nil {
		roles = []domain.Role{}
	}
	return domain.ListRolesResponse{Roles: roles}, nil
}

func (s *grantService) CandidatesForPrivilege(ctx context.Context, privilegeName string, teamIDs []int) (domain.GrantCandidatesResponse, error) {
	privilegeName = strings.TrimSpace(privilegeName)
	if privilegeName == "" {
		return domain.GrantCandidatesResponse{}, &apierror.ValidationError{Msg: "privilege is required"}
	}
	candidates, err := s.repo.CandidatesForPrivilege(ctx, privilegeName, teamIDs)
	if err != nil {
		return domain.GrantCandidatesResponse{}, err
	}
	if candidates == nil {
		candidates = []domain.GrantCandidate{}
	}
	return domain.GrantCandidatesResponse{Candidates: candidates}, nil
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

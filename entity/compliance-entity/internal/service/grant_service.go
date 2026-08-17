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

// GrantService reads and writes role grants.
//
// Deliberately has no cached counterpart, unlike UserService. Grants are
// authorisation data on the revocation path: an admin who removes a grant must
// see it take effect on the caller's next request. A TTL cache here would mean
// "revoked within N minutes, on most replicas", which is not something that can
// be stated honestly in a security review.
type GrantService interface {
	GrantsForUserID(ctx context.Context, userID int) (domain.UserGrantsResponse, error)
	GrantsForUserEmail(ctx context.Context, email string) (domain.UserGrantsResponse, error)
	CreateGrant(ctx context.Context, userID int, req domain.CreateUserGrantRequest) (domain.UserGrant, error)
	RevokeGrant(ctx context.Context, userID, grantID int) error
	ListRoles(ctx context.Context) (domain.ListRolesResponse, error)
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

func (s *grantService) GrantsForUserEmail(ctx context.Context, email string) (domain.UserGrantsResponse, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return domain.UserGrantsResponse{}, &apierror.ValidationError{Msg: "email is required"}
	}
	userID, grants, err := s.repo.GrantsForUserEmail(ctx, email)
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

	if err := s.validateScope(ctx, role, &req); err != nil {
		return domain.UserGrant{}, err
	}

	grant, err := s.repo.CreateGrant(ctx, userID, req)
	if err != nil {
		return domain.UserGrant{}, err
	}
	return *grant, nil
}

// validateScope enforces the three rules the schema cannot, mutating req's
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

	// 2. Grant-management is never handed out scoped. A register-scoped admin
	// granting roles would need the escalation rule that a grantor may not
	// exceed their own privileges in that scope — subtle, and silent when
	// wrong — so it is deliberately not permitted yet. Relaxing this later
	// needs no schema change.
	if req.ScopeType != domain.ScopeGlobal {
		carries, err := s.repo.RoleCarriesPrivilege(ctx, role.ID, manageUsersPrivilege)
		if err != nil {
			return err
		}
		if carries {
			return &apierror.ValidationError{
				Msg: role.RoleName + " carries " + manageUsersPrivilege +
					" and may only be granted GLOBAL"}
		}
	}

	// 3. The scope must exist. GLOBAL is a wildcard and takes the sentinel;
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

func (s *grantService) RevokeGrant(ctx context.Context, userID, grantID int) error {
	if userID <= 0 {
		return &apierror.ValidationError{Msg: "user id must be a positive integer"}
	}
	if grantID <= 0 {
		return &apierror.ValidationError{Msg: "grant id must be a positive integer"}
	}
	return s.repo.RevokeGrant(ctx, userID, grantID)
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

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

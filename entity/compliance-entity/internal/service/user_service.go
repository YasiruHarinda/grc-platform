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

type userService struct{ repo repository.UserRepository }

// NewUserService constructs a UserService.
func NewUserService(repo repository.UserRepository) UserService { return &userService{repo: repo} }

var validUserStatuses = map[string]bool{"ACTIVE": true, "INACTIVE": true, "REMOVED": true}

func (s *userService) SearchUsers(ctx context.Context, req domain.SearchUsersRequest) (domain.SearchUsersResponse, error) {
	if req.StatusKey != "" && !validUserStatuses[strings.ToUpper(req.StatusKey)] {
		return domain.SearchUsersResponse{}, &apierror.ValidationError{Msg: "invalid statusKey: must be ACTIVE, INACTIVE, or REMOVED"}
	}
	normalizePagination(&req.Pagination)
	users, total, err := s.repo.SearchUsers(ctx, req)
	if err != nil {
		return domain.SearchUsersResponse{}, err
	}
	if users == nil {
		users = []domain.User{}
	}
	return domain.SearchUsersResponse{Users: users, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *userService) GetUserByID(ctx context.Context, id int) (domain.User, error) {
	if id <= 0 {
		return domain.User{}, &apierror.ValidationError{Msg: "user id must be a positive integer"}
	}
	u, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	return *u, nil
}

func (s *userService) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	if email == "" {
		return domain.User{}, &apierror.ValidationError{Msg: "email is required"}
	}
	u, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, err
	}
	return *u, nil
}

func (s *userService) GetUserByUUID(ctx context.Context, uuid string) (domain.User, error) {
	if strings.TrimSpace(uuid) == "" {
		return domain.User{}, &apierror.ValidationError{Msg: "uuid is required"}
	}
	u, err := s.repo.GetUserByUUID(ctx, uuid)
	if err != nil {
		return domain.User{}, err
	}
	return *u, nil
}

func (s *userService) CreateUser(ctx context.Context, req domain.CreateUserRequest) (domain.User, error) {
	// Either identity anchor is enough: an HR-resolved employee is keyed by
	// email (uuid may still be unknown), while the admin console's "Add User"
	// provisions by uuid alone (email is being phased out — see the comment on
	// CreateUserRequest.Email). Refusing both-empty is the only thing this
	// still guards against.
	if req.Email == "" && strings.TrimSpace(req.UUID) == "" {
		return domain.User{}, &apierror.ValidationError{Msg: "email or uuid is required"}
	}
	// DisplayName is deliberately not required: callers are moving off storing
	// one at all (see risk/handler's resolve.go, which now sends "" here on
	// purpose) — requiring it would refuse the very provisioning flow that
	// migration depends on.
	if req.CreatedBy == "" {
		return domain.User{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	u, err := s.repo.CreateUser(ctx, req)
	if err != nil {
		return domain.User{}, err
	}
	return *u, nil
}

func (s *userService) UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (domain.User, error) {
	if id <= 0 {
		return domain.User{}, &apierror.ValidationError{Msg: "user id must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.User{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	u, err := s.repo.UpdateUser(ctx, id, req)
	if err != nil {
		return domain.User{}, err
	}
	return *u, nil
}

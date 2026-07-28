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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// PrivilegeService exposes the role→privilege mapping.
type PrivilegeService interface {
	RolePrivilegeMap(ctx context.Context) (domain.RolePrivilegeMapResponse, error)
}

type privilegeService struct {
	repo repository.PrivilegeRepository
}

// NewPrivilegeService constructs a PrivilegeService.
func NewPrivilegeService(repo repository.PrivilegeRepository) PrivilegeService {
	return &privilegeService{repo: repo}
}

func (s *privilegeService) RolePrivilegeMap(ctx context.Context) (domain.RolePrivilegeMapResponse, error) {
	m, err := s.repo.RolePrivilegeMap(ctx)
	if err != nil {
		return domain.RolePrivilegeMapResponse{}, err
	}
	// An empty map serialises as {} rather than null: a caller must be able to
	// tell "no grants configured" from a malformed response.
	if m == nil {
		m = map[string][]string{}
	}
	return domain.RolePrivilegeMapResponse{RolePrivileges: m}, nil
}

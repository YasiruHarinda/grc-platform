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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

// RiskScoreService defines business operations for risk score configurations.
type RiskScoreService interface {
	List(ctx context.Context) ([]*model.RiskScore, error)
	Create(ctx context.Context, req model.CreateRiskScoreRequest, createdBy string) (*model.RiskScore, error)
	Update(ctx context.Context, id int, req model.UpdateRiskScoreRequest, updatedBy string) error
}

type riskScoreService struct {
	repo repository.RiskScoreRepository
}

func NewRiskScoreService(repo repository.RiskScoreRepository) RiskScoreService {
	return &riskScoreService{repo: repo}
}

func (s *riskScoreService) List(ctx context.Context) ([]*model.RiskScore, error) {
	return s.repo.List(ctx)
}

func (s *riskScoreService) Create(ctx context.Context, req model.CreateRiskScoreRequest, createdBy string) (*model.RiskScore, error) {
	// TODO: validate likelihood/impact combination is unique, delegate to repo
	return nil, nil
}

func (s *riskScoreService) Update(ctx context.Context, id int, req model.UpdateRiskScoreRequest, updatedBy string) error {
	// TODO: delegate to repo
	return nil
}

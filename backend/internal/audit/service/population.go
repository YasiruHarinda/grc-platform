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
	"io"

	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-platform/backend/internal/shared/file"
)

// PopulationService defines business operations for OE control population files.
type PopulationService interface {
	List(ctx context.Context, controlID int) ([]*model.AuditPopulation, error)
	Upload(ctx context.Context, controlID int, fileName, contentType string, content io.Reader, createdBy string) (*model.AuditPopulation, error)
	Delete(ctx context.Context, controlID, populationID int, byUserID string) error
}

type populationService struct {
	repo    repository.PopulationRepository
	storage *file.Service
}

func NewPopulationService(repo repository.PopulationRepository, storage *file.Service) PopulationService {
	return &populationService{repo: repo, storage: storage}
}

func (s *populationService) List(ctx context.Context, controlID int) ([]*model.AuditPopulation, error) {
	// TODO: delegate to repo
	return nil, nil
}

func (s *populationService) Upload(ctx context.Context, controlID int, fileName, contentType string, content io.Reader, createdBy string) (*model.AuditPopulation, error) {
	// TODO: upload file via storage.Upload, upsert population row + evidence_file row via repo
	return nil, nil
}

func (s *populationService) Delete(ctx context.Context, controlID, populationID int, byUserID string) error {
	// TODO: fetch population, delete blob via storage.Delete, delete row via repo
	return nil
}

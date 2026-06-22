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

	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/audit/repository"
)

// FrameworkService defines business operations for audit frameworks and products.
type FrameworkService interface {
	ListFrameworks(ctx context.Context) ([]*model.AuditFramework, error)
	CreateFramework(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error)
	ListProducts(ctx context.Context) ([]*model.AuditProduct, error)
	CreateProduct(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error)
}

type frameworkService struct {
	frameworkRepo repository.FrameworkRepository
	productRepo   repository.ProductRepository
}

func NewFrameworkService(frameworkRepo repository.FrameworkRepository, productRepo repository.ProductRepository) FrameworkService {
	return &frameworkService{frameworkRepo: frameworkRepo, productRepo: productRepo}
}

func (s *frameworkService) ListFrameworks(ctx context.Context) ([]*model.AuditFramework, error) {
	// TODO: delegate to frameworkRepo
	return nil, nil
}

func (s *frameworkService) CreateFramework(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error) {
	// TODO: validate name uniqueness, delegate to frameworkRepo
	return nil, nil
}

func (s *frameworkService) ListProducts(ctx context.Context) ([]*model.AuditProduct, error) {
	// TODO: delegate to productRepo
	return nil, nil
}

func (s *frameworkService) CreateProduct(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error) {
	// TODO: validate name uniqueness, delegate to productRepo
	return nil, nil
}

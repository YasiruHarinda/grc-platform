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
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// FrameworkService defines business operations for audit frameworks, products,
// and the versioned framework control library.
type FrameworkService interface {
	ListFrameworks(ctx context.Context) ([]*model.AuditFramework, error)
	CreateFramework(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error)
	ListProducts(ctx context.Context) ([]*model.AuditProduct, error)
	CreateProduct(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error)
	ListFrameworkControls(ctx context.Context, frameworkID int) ([]*model.AuditFrameworkControl, error)
	CreateFrameworkControl(ctx context.Context, frameworkID int, req model.CreateFrameworkControlRequest, createdBy string) (*model.AuditFrameworkControl, error)
}

type frameworkService struct {
	frameworkRepo        repository.FrameworkRepository
	productRepo          repository.ProductRepository
	frameworkControlRepo repository.FrameworkControlRepository
}

func NewFrameworkService(frameworkRepo repository.FrameworkRepository, productRepo repository.ProductRepository, frameworkControlRepo repository.FrameworkControlRepository) FrameworkService {
	return &frameworkService{frameworkRepo: frameworkRepo, productRepo: productRepo, frameworkControlRepo: frameworkControlRepo}
}

func (s *frameworkService) ListFrameworks(ctx context.Context) ([]*model.AuditFramework, error) {
	return s.frameworkRepo.List(ctx)
}

func (s *frameworkService) CreateFramework(ctx context.Context, req model.CreateFrameworkRequest, createdBy string) (*model.AuditFramework, error) {
	if req.Name == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "name is required"}
	}
	return s.frameworkRepo.Create(ctx, req, createdBy)
}

func (s *frameworkService) ListProducts(ctx context.Context) ([]*model.AuditProduct, error) {
	return s.productRepo.List(ctx)
}

func (s *frameworkService) CreateProduct(ctx context.Context, req model.CreateProductRequest, createdBy string) (*model.AuditProduct, error) {
	if req.Name == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "name is required"}
	}
	return s.productRepo.Create(ctx, req, createdBy)
}

func (s *frameworkService) ListFrameworkControls(ctx context.Context, frameworkID int) ([]*model.AuditFrameworkControl, error) {
	return s.frameworkControlRepo.ListCurrent(ctx, frameworkID)
}

func (s *frameworkService) CreateFrameworkControl(ctx context.Context, frameworkID int, req model.CreateFrameworkControlRequest, createdBy string) (*model.AuditFrameworkControl, error) {
	if req.ControlNumber == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "controlNumber is required"}
	}
	if req.Description == "" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "description is required"}
	}
	if req.RequirementType != "DESIGN" && req.RequirementType != "OE" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "requirementType must be DESIGN or OE"}
	}
	if req.ControlType != "CONFIG" && req.ControlType != "NON_CONFIG" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "controlType must be CONFIG or NON_CONFIG"}
	}
	if req.Scope != "COMMON" && req.Scope != "PRODUCT_SPECIFIC" {
		return nil, &apierror.Error{StatusCode: http.StatusUnprocessableEntity, Body: "scope must be COMMON or PRODUCT_SPECIFIC"}
	}
	return s.frameworkControlRepo.Create(ctx, frameworkID, req, createdBy)
}

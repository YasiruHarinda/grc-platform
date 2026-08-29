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

// AdminActivityLogService defines business operations for the Admin Console's
// activity log (append-only, mirrors AuditTrailService).
type AdminActivityLogService interface {
	CreateAdminActivityLog(ctx context.Context, req domain.CreateAdminActivityLogRequest) (domain.AdminActivityLog, error)
	ListAdminActivityLog(ctx context.Context, filter domain.AdminActivityLogFilter, limit, offset int) (domain.ListAdminActivityLogResponse, error)
}

type adminActivityLogService struct {
	repo repository.AdminActivityLogRepository
}

// NewAdminActivityLogService constructs an AdminActivityLogService.
func NewAdminActivityLogService(repo repository.AdminActivityLogRepository) AdminActivityLogService {
	return &adminActivityLogService{repo: repo}
}

// validAdminActivityActions/validAdminActivityEntityTypes mirror the
// admin_activity_log.action/entity_type ENUM columns exactly (see shared.sql).
var validAdminActivityActions = map[string]bool{
	"CREATED": true, "UPDATED": true, "DELETED": true,
	"STATUS_CHANGED": true, "GRANTED": true, "REVOKED": true,
}

var validAdminActivityEntityTypes = map[string]bool{
	"USER": true, "GRANT": true, "RISK_TEAM": true, "RISK_CATEGORY": true,
	"COMPLIANCE_REFERENCE": true, "RISK_SCORE": true, "AUDIT_TEAM": true,
}

func (s *adminActivityLogService) CreateAdminActivityLog(ctx context.Context, req domain.CreateAdminActivityLogRequest) (domain.AdminActivityLog, error) {
	if strings.TrimSpace(req.ActorID) == "" {
		return domain.AdminActivityLog{}, &apierror.ValidationError{Msg: "actorId is required"}
	}
	req.Action = strings.ToUpper(req.Action)
	if !validAdminActivityActions[req.Action] {
		return domain.AdminActivityLog{}, &apierror.ValidationError{Msg: "invalid action: " + req.Action}
	}
	req.EntityType = strings.ToUpper(req.EntityType)
	if !validAdminActivityEntityTypes[req.EntityType] {
		return domain.AdminActivityLog{}, &apierror.ValidationError{Msg: "invalid entityType: " + req.EntityType}
	}
	if req.EntityID <= 0 {
		return domain.AdminActivityLog{}, &apierror.ValidationError{Msg: "entityId must be a positive integer"}
	}
	if err := validJSONField("details", req.Details); err != nil {
		return domain.AdminActivityLog{}, err
	}
	e, err := s.repo.CreateAdminActivityLog(ctx, req)
	if err != nil {
		return domain.AdminActivityLog{}, err
	}
	return *e, nil
}

func (s *adminActivityLogService) ListAdminActivityLog(ctx context.Context, filter domain.AdminActivityLogFilter, limit, offset int) (domain.ListAdminActivityLogResponse, error) {
	if filter.Action != "" {
		filter.Action = strings.ToUpper(filter.Action)
		if !validAdminActivityActions[filter.Action] {
			return domain.ListAdminActivityLogResponse{}, &apierror.ValidationError{Msg: "invalid action: " + filter.Action}
		}
	}
	if filter.EntityType != "" {
		filter.EntityType = strings.ToUpper(filter.EntityType)
		if !validAdminActivityEntityTypes[filter.EntityType] {
			return domain.ListAdminActivityLogResponse{}, &apierror.ValidationError{Msg: "invalid entityType: " + filter.EntityType}
		}
	}
	p := domain.Pagination{Limit: limit, Offset: offset}
	normalizePagination(&p)
	entries, total, err := s.repo.ListAdminActivityLog(ctx, filter, p.Limit, p.Offset)
	if err != nil {
		return domain.ListAdminActivityLogResponse{}, err
	}
	if entries == nil {
		entries = []domain.AdminActivityLog{}
	}
	return domain.ListAdminActivityLogResponse{Entries: entries, Total: total, Limit: p.Limit, Offset: p.Offset}, nil
}

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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type auditNotificationService struct {
	repo repository.AuditNotificationRepository
}

// NewAuditNotificationService constructs an AuditNotificationService.
func NewAuditNotificationService(repo repository.AuditNotificationRepository) AuditNotificationService {
	return &auditNotificationService{repo: repo}
}

var validNotificationTypes = map[string]bool{
	"OWNER_ASSIGNED_CONTROL":    true,
	"OWNER_ASSIGNED_POPULATION": true,
	"REMINDER_DUE_10":           true,
	"REMINDER_DUE_5":            true,
	"REMINDER_OVERDUE":          true,
	"RESUBMISSION_NEEDED":       true,
	"SAMPLE_SUBMITTED":          true,
}

func (s *auditNotificationService) CreateAuditNotification(ctx context.Context, req domain.CreateAuditNotificationRequest) (domain.AuditNotification, error) {
	if req.RecipientID <= 0 {
		return domain.AuditNotification{}, &apierror.ValidationError{Msg: "recipientId must be a positive integer"}
	}
	if !validNotificationTypes[req.Type] {
		return domain.AuditNotification{}, &apierror.ValidationError{Msg: "invalid type: " + req.Type}
	}
	if (req.ControlID == nil) == (req.PopulationID == nil) {
		return domain.AuditNotification{}, &apierror.ValidationError{Msg: "exactly one of controlId or populationId is required"}
	}
	n, err := s.repo.CreateAuditNotification(ctx, req)
	if err != nil {
		return domain.AuditNotification{}, err
	}
	return *n, nil
}

func (s *auditNotificationService) AuditNotificationExists(ctx context.Context, req domain.AuditNotificationExistsRequest) (bool, error) {
	if req.RecipientID <= 0 {
		return false, &apierror.ValidationError{Msg: "recipientId must be a positive integer"}
	}
	if !validNotificationTypes[req.Type] {
		return false, &apierror.ValidationError{Msg: "invalid type: " + req.Type}
	}
	if (req.ControlID == nil) == (req.PopulationID == nil) {
		return false, &apierror.ValidationError{Msg: "exactly one of controlId or populationId is required"}
	}
	return s.repo.AuditNotificationExists(ctx, req)
}

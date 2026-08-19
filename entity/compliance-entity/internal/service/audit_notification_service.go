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
	"AUDITOR_ASSIGNED_CONTROL":  true,
	"REMINDER_DUE_10":           true,
	"REMINDER_DUE_5":            true,
	"REMINDER_OVERDUE":          true,
	"RESUBMISSION_NEEDED":       true,
	"SAMPLE_SUBMITTED":          true,
}

// validReminderTypes is the subset of validNotificationTypes that claim/
// release apply to — the other five types are single-event triggers with no
// overlapping-run problem to guard against.
var validReminderTypes = map[string]bool{
	"REMINDER_DUE_10":  true,
	"REMINDER_DUE_5":   true,
	"REMINDER_OVERDUE": true,
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

func (s *auditNotificationService) ClaimAuditNotification(ctx context.Context, req domain.ClaimAuditNotificationRequest) (bool, int64, error) {
	if req.RecipientID <= 0 {
		return false, 0, &apierror.ValidationError{Msg: "recipientId must be a positive integer"}
	}
	if !validReminderTypes[req.Type] {
		return false, 0, &apierror.ValidationError{Msg: "invalid type for a reminder claim: " + req.Type}
	}
	if (req.ControlID == nil) == (req.PopulationID == nil) {
		return false, 0, &apierror.ValidationError{Msg: "exactly one of controlId or populationId is required"}
	}
	n, claimed, err := s.repo.ClaimAuditNotification(ctx, req)
	if err != nil || !claimed {
		return false, 0, err
	}
	return true, n.ID, nil
}

func (s *auditNotificationService) ReleaseAuditNotificationClaim(ctx context.Context, id int64) error {
	if id <= 0 {
		return &apierror.ValidationError{Msg: "id must be a positive integer"}
	}
	return s.repo.ReleaseAuditNotificationClaim(ctx, id)
}

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

type riskNotificationService struct {
	repo repository.RiskNotificationRepository
}

// NewRiskNotificationService constructs a RiskNotificationService.
func NewRiskNotificationService(repo repository.RiskNotificationRepository) RiskNotificationService {
	return &riskNotificationService{repo: repo}
}

var validNotificationTypes = map[string]bool{
	"REMINDER": true, "ESCALATION": true, "STATUS_CHANGE": true,
	"APPROVAL": true, "REASSESSMENT": true, "REJECTION": true,
}
var validNotificationChannels = map[string]bool{"EMAIL": true, "IN_APP": true}

func (s *riskNotificationService) CreateRiskNotification(ctx context.Context, req domain.CreateRiskNotificationRequest) (domain.RiskNotification, error) {
	if req.RecipientID <= 0 {
		return domain.RiskNotification{}, &apierror.ValidationError{Msg: "recipientId must be a positive integer"}
	}
	req.Type = strings.ToUpper(req.Type)
	if !validNotificationTypes[req.Type] {
		return domain.RiskNotification{}, &apierror.ValidationError{Msg: "invalid type: " + req.Type}
	}
	if req.Channel != nil {
		up := strings.ToUpper(*req.Channel)
		if !validNotificationChannels[up] {
			return domain.RiskNotification{}, &apierror.ValidationError{Msg: "channel must be EMAIL or IN_APP"}
		}
		req.Channel = &up
	}
	if req.Message == "" {
		return domain.RiskNotification{}, &apierror.ValidationError{Msg: "message is required"}
	}
	if req.CreatedBy == "" {
		return domain.RiskNotification{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	n, err := s.repo.CreateRiskNotification(ctx, req)
	if err != nil {
		return domain.RiskNotification{}, err
	}
	return *n, nil
}

func (s *riskNotificationService) ListRiskNotifications(ctx context.Context, recipientID int) (domain.ListRiskNotificationsResponse, error) {
	if recipientID <= 0 {
		return domain.ListRiskNotificationsResponse{}, &apierror.ValidationError{Msg: "recipientId must be a positive integer"}
	}
	notifications, err := s.repo.ListRiskNotifications(ctx, recipientID)
	if err != nil {
		return domain.ListRiskNotificationsResponse{}, err
	}
	if notifications == nil {
		notifications = []domain.RiskNotification{}
	}
	return domain.ListRiskNotificationsResponse{Notifications: notifications}, nil
}

func (s *riskNotificationService) MarkRiskNotificationRead(ctx context.Context, id int64, req domain.MarkRiskNotificationReadRequest) (domain.RiskNotification, error) {
	if id <= 0 {
		return domain.RiskNotification{}, &apierror.ValidationError{Msg: "notification id must be a positive integer"}
	}
	if req.RecipientID <= 0 {
		return domain.RiskNotification{}, &apierror.ValidationError{Msg: "recipientId must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.RiskNotification{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	n, err := s.repo.MarkRiskNotificationRead(ctx, id, req)
	if err != nil {
		return domain.RiskNotification{}, err
	}
	return *n, nil
}

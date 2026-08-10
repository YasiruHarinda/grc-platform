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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
)

// NotificationService is the send-log for every audit-module email
// (assignment, reminder, resubmission, sample-submitted) and the daily
// reminder job's de-dup mechanism — a thin pass-through over
// repository.NotificationRepository. See model.NotificationLogEntry and
// audit_schema.sql's audit_notification comment for the full design.
type NotificationService interface {
	// Create logs one sent email. Called only after a successful send.
	Create(ctx context.Context, n model.NotificationLogEntry) error
	// Exists reports whether a matching (recipient, type, control/population,
	// due date) row already exists — the reminder job's de-dup check.
	Exists(ctx context.Context, recipientID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (bool, error)
}

type notificationService struct {
	repo repository.NotificationRepository
}

func NewNotificationService(repo repository.NotificationRepository) NotificationService {
	return &notificationService{repo: repo}
}

func (s *notificationService) Create(ctx context.Context, n model.NotificationLogEntry) error {
	return s.repo.Create(ctx, n)
}

func (s *notificationService) Exists(ctx context.Context, recipientID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (bool, error) {
	return s.repo.Exists(ctx, recipientID, notifType, controlID, populationID, dueDateSnapshot)
}

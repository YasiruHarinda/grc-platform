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
	// Create logs one sent email. Called only after a successful send — used
	// by every notification type except the reminder tiers, which use
	// Claim/ReleaseClaim instead.
	Create(ctx context.Context, n model.NotificationLogEntry) error
	// Claim atomically reserves a reminder item. claimed=false means another
	// caller already claimed it; not an error.
	Claim(ctx context.Context, recipientID, auditID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (claimed bool, notificationID int64, err error)
	// ReleaseClaim deletes a claim row so its item is retryable on a future
	// run — called only when the owner's digest failed to send after the
	// claim succeeded.
	ReleaseClaim(ctx context.Context, notificationID int64) error
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

func (s *notificationService) Claim(ctx context.Context, recipientID, auditID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (bool, int64, error) {
	return s.repo.Claim(ctx, recipientID, auditID, notifType, controlID, populationID, dueDateSnapshot)
}

func (s *notificationService) ReleaseClaim(ctx context.Context, notificationID int64) error {
	return s.repo.ReleaseClaim(ctx, notificationID)
}

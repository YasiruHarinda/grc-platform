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

// NotificationService defines business operations for audit notifications.
type NotificationService interface {
	List(ctx context.Context, recipientID int) ([]*model.AuditNotification, error)
	MarkRead(ctx context.Context, id, recipientID int) error
}

type notificationService struct {
	repo repository.NotificationRepository
}

func NewNotificationService(repo repository.NotificationRepository) NotificationService {
	return &notificationService{repo: repo}
}

func (s *notificationService) List(ctx context.Context, recipientID int) ([]*model.AuditNotification, error) {
	// TODO: delegate to repo, filter by recipient_id
	return nil, nil
}

func (s *notificationService) MarkRead(ctx context.Context, id, recipientID int) error {
	// TODO: verify notification belongs to recipientID, set is_read=true via repo
	return nil
}

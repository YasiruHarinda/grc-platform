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

package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type notificationRepo struct{ c *entityclient.Client }

// NewNotificationRepository returns an entity-backed NotificationRepository.
func NewNotificationRepository(c *entityclient.Client) repository.NotificationRepository {
	return &notificationRepo{c: c}
}

func (r *notificationRepo) Create(ctx context.Context, n model.NotificationLogEntry) error {
	body := map[string]any{
		"recipientId":     n.RecipientID,
		"auditId":         n.AuditID,
		"controlId":       n.ControlID,
		"populationId":    n.PopulationID,
		"type":            n.Type,
		"dueDateSnapshot": n.DueDateSnapshot,
		"message":         n.Message,
		"createdBy":       "system",
	}
	return r.c.Post(ctx, "/audit/notifications", body, nil)
}

func (r *notificationRepo) Claim(ctx context.Context, recipientID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (bool, int64, error) {
	var resp struct {
		Claimed bool  `json:"claimed"`
		ID      int64 `json:"id"`
	}
	body := map[string]any{
		"recipientId":     recipientID,
		"type":            notifType,
		"controlId":       controlID,
		"populationId":    populationID,
		"dueDateSnapshot": dueDateSnapshot,
	}
	if err := r.c.Post(ctx, "/audit/notifications/claim", body, &resp); err != nil {
		return false, 0, err
	}
	return resp.Claimed, resp.ID, nil
}

func (r *notificationRepo) ReleaseClaim(ctx context.Context, notificationID int64) error {
	return r.c.Delete(ctx, fmt.Sprintf("/audit/notifications/%d/claim", notificationID))
}

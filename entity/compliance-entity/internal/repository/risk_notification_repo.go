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

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// RiskNotificationRepository defines persistence for risk_notification.
type RiskNotificationRepository interface {
	CreateRiskNotification(ctx context.Context, req domain.CreateRiskNotificationRequest) (*domain.RiskNotification, error)
	ListRiskNotifications(ctx context.Context, recipientID int) ([]domain.RiskNotification, error)
	MarkRiskNotificationRead(ctx context.Context, id int64, req domain.MarkRiskNotificationReadRequest) (*domain.RiskNotification, error)
}

type riskNotificationRepo struct{ db *sql.DB }

// NewRiskNotificationRepository constructs a RiskNotificationRepository.
func NewRiskNotificationRepository(db *sql.DB) RiskNotificationRepository {
	return &riskNotificationRepo{db: db}
}

func (r *riskNotificationRepo) CreateRiskNotification(ctx context.Context, req domain.CreateRiskNotificationRequest) (*domain.RiskNotification, error) {
	channel := "IN_APP"
	if req.Channel != nil && *req.Channel != "" {
		channel = *req.Channel
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_notification (recipient_id, risk_id, type, channel, message, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.RecipientID, nullableInt(req.RiskID), req.Type, channel, req.Message, req.CreatedBy, req.CreatedBy,
	)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: "recipient or risk not found"}
		}
		return nil, fmt.Errorf("risk_notification.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getByID(ctx, id)
}

func (r *riskNotificationRepo) getByID(ctx context.Context, id int64) (*domain.RiskNotification, error) {
	n, err := scanRiskNotification(r.db.QueryRowContext(ctx,
		`SELECT id, recipient_id, risk_id, type, channel, message, is_read, created_by, updated_by, created_at, updated_at
		 FROM risk_notification WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("notification %d not found", id)}
	}
	if err != nil {
		return nil, fmt.Errorf("risk_notification.GetByID(%d): %w", id, err)
	}
	return n, nil
}

func (r *riskNotificationRepo) ListRiskNotifications(ctx context.Context, recipientID int) ([]domain.RiskNotification, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, recipient_id, risk_id, type, channel, message, is_read, created_by, updated_by, created_at, updated_at
		 FROM risk_notification WHERE recipient_id = ? ORDER BY created_at DESC`, recipientID)
	if err != nil {
		return nil, fmt.Errorf("risk_notification.List: %w", err)
	}
	defer rows.Close()

	var notifications []domain.RiskNotification
	for rows.Next() {
		n, err := scanRiskNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("risk_notification.List scan: %w", err)
		}
		notifications = append(notifications, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("risk_notification.List rows: %w", err)
	}
	return notifications, nil
}

func (r *riskNotificationRepo) MarkRiskNotificationRead(ctx context.Context, id int64, req domain.MarkRiskNotificationReadRequest) (*domain.RiskNotification, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE risk_notification SET is_read = TRUE, updated_by = ? WHERE id = ? AND recipient_id = ?`,
		req.UpdatedBy, id, req.RecipientID)
	if err != nil {
		return nil, fmt.Errorf("risk_notification.MarkRead(%d): %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("notification %d not found for recipient %d", id, req.RecipientID)}
	}
	return r.getByID(ctx, id)
}

func scanRiskNotification(s scanner) (*domain.RiskNotification, error) {
	var n domain.RiskNotification
	var riskID sql.NullInt64
	var createdBy, updatedBy sql.NullString
	err := s.Scan(
		&n.ID, &n.RecipientID, &riskID, &n.Type, &n.Channel, &n.Message, &n.IsRead,
		&createdBy, &updatedBy, &n.CreatedOn, &n.UpdatedOn,
	)
	if err != nil {
		return nil, err
	}
	if riskID.Valid {
		v := int(riskID.Int64)
		n.RiskID = &v
	}
	if createdBy.Valid {
		n.CreatedBy = &createdBy.String
	}
	if updatedBy.Valid {
		n.UpdatedBy = &updatedBy.String
	}
	return &n, nil
}

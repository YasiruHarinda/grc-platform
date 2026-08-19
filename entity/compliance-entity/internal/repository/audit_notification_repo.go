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
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// AuditNotificationRepository defines persistence for audit_notification.
type AuditNotificationRepository interface {
	CreateAuditNotification(ctx context.Context, req domain.CreateAuditNotificationRequest) (*domain.AuditNotification, error)
	// AuditNotificationExists is the reminder job's de-dup check — see
	// audit_schema.sql's audit_notification comment for why this is an
	// application-level NULL-safe existence check rather than a DB unique
	// constraint.
	AuditNotificationExists(ctx context.Context, req domain.AuditNotificationExistsRequest) (bool, error)
}

type auditNotificationRepo struct{ db *sql.DB }

// NewAuditNotificationRepository constructs an AuditNotificationRepository.
func NewAuditNotificationRepository(db *sql.DB) AuditNotificationRepository {
	return &auditNotificationRepo{db: db}
}

func (r *auditNotificationRepo) CreateAuditNotification(ctx context.Context, req domain.CreateAuditNotificationRequest) (*domain.AuditNotification, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_notification
		 (recipient_id, audit_id, control_id, population_id, type, due_date_snapshot, message, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.RecipientID,
		nullableInt(req.AuditID),
		nullableInt(req.ControlID),
		nullableInt(req.PopulationID),
		req.Type,
		nullableString(req.DueDateSnapshot),
		nullableString(req.Message),
		nullableString(req.CreatedBy),
	)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("recipient %d not found", req.RecipientID)}
		}
		return nil, fmt.Errorf("audit_notification.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("audit_notification.Create last insert id: %w", err)
	}
	return r.getAuditNotificationByID(ctx, id)
}

func (r *auditNotificationRepo) getAuditNotificationByID(ctx context.Context, id int64) (*domain.AuditNotification, error) {
	return scanAuditNotification(r.db.QueryRowContext(ctx,
		`SELECT id, recipient_id, audit_id, control_id, population_id, type,
		        channel, due_date_snapshot, message, created_by, created_at
		 FROM audit_notification WHERE id = ?`, id))
}

// AuditNotificationExists uses NULL-safe equality (<=>) on control_id/
// population_id/due_date_snapshot so the check works correctly regardless of
// which of control_id/population_id is set for this notification type.
func (r *auditNotificationRepo) AuditNotificationExists(ctx context.Context, req domain.AuditNotificationExistsRequest) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM audit_notification
		 WHERE recipient_id = ? AND type = ?
		   AND control_id <=> ? AND population_id <=> ? AND due_date_snapshot <=> ?
		 LIMIT 1`,
		req.RecipientID, req.Type,
		nullableInt(req.ControlID), nullableInt(req.PopulationID), nullableString(req.DueDateSnapshot),
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("audit_notification.Exists: %w", err)
	}
	return true, nil
}

func scanAuditNotification(s scanner) (*domain.AuditNotification, error) {
	var n domain.AuditNotification
	var recipientID, auditID, controlID, populationID sql.NullInt64
	var dueDateSnapshot, message, createdBy sql.NullString
	err := s.Scan(
		&n.ID, &recipientID, &auditID, &controlID, &populationID, &n.Type,
		&n.Channel, &dueDateSnapshot, &message, &createdBy, &n.CreatedOn,
	)
	if err != nil {
		return nil, err
	}
	if recipientID.Valid {
		v := int(recipientID.Int64)
		n.RecipientID = &v
	}
	if auditID.Valid {
		v := int(auditID.Int64)
		n.AuditID = &v
	}
	if controlID.Valid {
		v := int(controlID.Int64)
		n.ControlID = &v
	}
	if populationID.Valid {
		v := int(populationID.Int64)
		n.PopulationID = &v
	}
	if dueDateSnapshot.Valid {
		n.DueDateSnapshot = &dueDateSnapshot.String
	}
	if message.Valid {
		n.Message = &message.String
	}
	if createdBy.Valid {
		n.CreatedBy = &createdBy.String
	}
	return &n, nil
}

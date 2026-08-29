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
	// ClaimAuditNotification is the reminder job's atomic de-dup claim — see
	// audit_schema.sql's audit_notification comment.
	ClaimAuditNotification(ctx context.Context, req domain.ClaimAuditNotificationRequest) (*domain.AuditNotification, bool, error)
	// ReleaseAuditNotificationClaim deletes a claim row so its item becomes
	// retryable on a future run — used only when the owner's digest failed to
	// send after the claim succeeded.
	ReleaseAuditNotificationClaim(ctx context.Context, id int64) error
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

// ClaimAuditNotification atomically inserts an audit_notification row for a
// reminder item, iff no row with the same reminder_dedup_key exists yet (see
// uq_notif_reminder_dedup and audit_schema.sql's audit_notification comment).
// The insert IS the claim: a caller that wins it is the sole owner of
// sending this item for this due_date_snapshot. Losing the race (someone else
// already claimed it) is reported as claimed=false, not an error.
func (r *auditNotificationRepo) ClaimAuditNotification(ctx context.Context, req domain.ClaimAuditNotificationRequest) (*domain.AuditNotification, bool, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_notification
		 (recipient_id, audit_id, control_id, population_id, type, due_date_snapshot, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, 'system')`,
		req.RecipientID, nullableInt(req.AuditID), nullableInt(req.ControlID),
		nullableInt(req.PopulationID), req.Type, nullableString(req.DueDateSnapshot),
	)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, false, nil
		}
		if isFKViolation(err) {
			return nil, false, &apierror.NotFoundError{Msg: fmt.Sprintf("recipient %d not found", req.RecipientID)}
		}
		return nil, false, fmt.Errorf("audit_notification.Claim: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, false, fmt.Errorf("audit_notification.Claim last insert id: %w", err)
	}
	n, err := r.getAuditNotificationByID(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return n, true, nil
}

// ReleaseAuditNotificationClaim deletes a claim row so its item becomes
// retryable — used only when the owner's digest failed to send after the
// claim succeeded. Scoped to the four REMINDER_* types so this can never be
// pointed at a delivered, non-reminder log row.
func (r *auditNotificationRepo) ReleaseAuditNotificationClaim(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM audit_notification
		 WHERE id = ? AND type IN ('REMINDER_DUE_10', 'REMINDER_DUE_5', 'REMINDER_DUE_TODAY', 'REMINDER_OVERDUE')`, id)
	if err != nil {
		return fmt.Errorf("audit_notification.ReleaseClaim: %w", err)
	}
	return nil
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

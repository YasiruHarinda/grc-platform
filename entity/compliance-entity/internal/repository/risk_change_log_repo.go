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

// RiskChangeLogRepository defines persistence for risk_change_log (append-only).
type RiskChangeLogRepository interface {
	CreateRiskChangeLog(ctx context.Context, riskID int, req domain.CreateRiskChangeLogRequest) (*domain.RiskChangeLog, error)
	ListRiskChangeLog(ctx context.Context, riskID int, limit, offset int) ([]domain.RiskChangeLog, int, error)
}

type riskChangeLogRepo struct{ db *sql.DB }

// NewRiskChangeLogRepository constructs a RiskChangeLogRepository.
func NewRiskChangeLogRepository(db *sql.DB) RiskChangeLogRepository {
	return &riskChangeLogRepo{db: db}
}

func (r *riskChangeLogRepo) CreateRiskChangeLog(ctx context.Context, riskID int, req domain.CreateRiskChangeLogRequest) (*domain.RiskChangeLog, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO risk_change_log (risk_id, created_by, action, field_changed, old_value, new_value, details)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		riskID,
		req.CreatedBy,
		req.Action,
		nullableString(req.FieldChanged),
		nullableString(req.OldValue),
		nullableString(req.NewValue),
		nullableString(req.Details),
	)
	if err != nil {
		if isFKViolation(err) {
			return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("risk %d not found", riskID)}
		}
		return nil, fmt.Errorf("risk_change_log.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return r.getByID(ctx, id)
}

func (r *riskChangeLogRepo) getByID(ctx context.Context, id int64) (*domain.RiskChangeLog, error) {
	return scanRiskChangeLog(r.db.QueryRowContext(ctx,
		`SELECT id, risk_id, created_by, action, field_changed, old_value, new_value, details, created_at
		 FROM risk_change_log WHERE id = ?`, id))
}

func (r *riskChangeLogRepo) ListRiskChangeLog(ctx context.Context, riskID int, limit, offset int) ([]domain.RiskChangeLog, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM risk_change_log WHERE risk_id = ?", riskID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("risk_change_log.ListCount: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, risk_id, created_by, action, field_changed, old_value, new_value, details, created_at
		 FROM risk_change_log WHERE risk_id = ?
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		riskID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("risk_change_log.List: %w", err)
	}
	defer rows.Close()

	var entries []domain.RiskChangeLog
	for rows.Next() {
		e, err := scanRiskChangeLog(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("risk_change_log.List scan: %w", err)
		}
		entries = append(entries, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("risk_change_log.List rows: %w", err)
	}
	return entries, total, nil
}

func scanRiskChangeLog(s scanner) (*domain.RiskChangeLog, error) {
	var e domain.RiskChangeLog
	var fieldChanged, oldValue, newValue, details sql.NullString
	err := s.Scan(
		&e.ID, &e.RiskID, &e.CreatedBy, &e.Action,
		&fieldChanged, &oldValue, &newValue, &details, &e.CreatedOn,
	)
	if err != nil {
		return nil, err
	}
	if fieldChanged.Valid {
		e.FieldChanged = &fieldChanged.String
	}
	if oldValue.Valid {
		e.OldValue = &oldValue.String
	}
	if newValue.Valid {
		e.NewValue = &newValue.String
	}
	if details.Valid {
		e.Details = &details.String
	}
	return &e, nil
}

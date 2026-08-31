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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// AdminActivityLogRepository defines persistence for admin_activity_log.
type AdminActivityLogRepository interface {
	CreateAdminActivityLog(ctx context.Context, req domain.CreateAdminActivityLogRequest) (*domain.AdminActivityLog, error)
	// ListAdminActivityLog returns entries newest first, narrowed by filter
	// (all fields optional), with the total count ignoring limit/offset.
	ListAdminActivityLog(ctx context.Context, filter domain.AdminActivityLogFilter, limit, offset int) ([]domain.AdminActivityLog, int, error)
}

type adminActivityLogRepo struct{ db *sql.DB }

// NewAdminActivityLogRepository constructs an AdminActivityLogRepository.
func NewAdminActivityLogRepository(db *sql.DB) AdminActivityLogRepository {
	return &adminActivityLogRepo{db: db}
}

func (r *adminActivityLogRepo) CreateAdminActivityLog(ctx context.Context, req domain.CreateAdminActivityLogRequest) (*domain.AdminActivityLog, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO admin_activity_log (actor_id, action, entity_type, entity_id, details)
		 VALUES (?, ?, ?, ?, ?)`,
		req.ActorID, req.Action, req.EntityType, req.EntityID, nullableString(req.Details),
	)
	if err != nil {
		return nil, fmt.Errorf("admin_activity_log.Create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("admin_activity_log.Create last insert id: %w", err)
	}
	return r.getAdminActivityLogByID(ctx, id)
}

const adminActivityLogSelectCols = `
	        t.id, t.actor_id, t.action, t.entity_type, t.entity_id,
	        t.details, u_actor.user_type AS actor_user_type, t.created_at`

const adminActivityLogFromClause = `
	 FROM admin_activity_log t
	 LEFT JOIN ` + "`user`" + ` u_actor ON u_actor.uuid = t.actor_id`

func (r *adminActivityLogRepo) getAdminActivityLogByID(ctx context.Context, id int64) (*domain.AdminActivityLog, error) {
	return scanAdminActivityLog(r.db.QueryRowContext(ctx,
		`SELECT`+adminActivityLogSelectCols+adminActivityLogFromClause+` WHERE t.id = ?`, id))
}

func (r *adminActivityLogRepo) ListAdminActivityLog(ctx context.Context, filter domain.AdminActivityLogFilter, limit, offset int) ([]domain.AdminActivityLog, int, error) {
	where := "WHERE 1=1"
	var args []any

	if filter.ActorID != "" {
		where += " AND t.actor_id = ?"
		args = append(args, filter.ActorID)
	}
	if filter.Action != "" {
		where += " AND t.action = ?"
		args = append(args, filter.Action)
	}
	if filter.EntityType != "" {
		where += " AND t.entity_type = ?"
		args = append(args, filter.EntityType)
	}
	// To is exclusive: the handler already shifted it to the next midnight, so
	// <= would also match a row created exactly at 00:00:00 the day after.
	where += " AND (? IS NULL OR t.created_at >= ?) AND (? IS NULL OR t.created_at < ?)"
	args = append(args,
		nilableAny(filter.From), nilableAny(filter.From),
		nilableAny(filter.To), nilableAny(filter.To),
	)

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM admin_activity_log t "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("admin_activity_log.ListCount: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT`+adminActivityLogSelectCols+adminActivityLogFromClause+" "+where+
			" ORDER BY t.created_at DESC, t.id DESC LIMIT ? OFFSET ?",
		append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("admin_activity_log.List: %w", err)
	}
	defer rows.Close()

	var entries []domain.AdminActivityLog
	for rows.Next() {
		e, err := scanAdminActivityLog(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("admin_activity_log.List scan: %w", err)
		}
		entries = append(entries, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("admin_activity_log.List rows: %w", err)
	}
	return entries, total, nil
}

func scanAdminActivityLog(s scanner) (*domain.AdminActivityLog, error) {
	var e domain.AdminActivityLog
	var details sql.NullString
	var actorUserType sql.NullString
	err := s.Scan(
		&e.ID, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID,
		&details, &actorUserType, &e.CreatedOn,
	)
	if err != nil {
		return nil, err
	}
	if details.Valid {
		e.Details = &details.String
	}
	if actorUserType.Valid {
		e.ActorUserType = actorUserType.String
	}
	return &e, nil
}

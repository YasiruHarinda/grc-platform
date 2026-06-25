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

package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/repository"
)

type teamRepository struct{ db *sql.DB }

// NewTeamRepository creates a MySQL-backed repository.TeamRepository.
func NewTeamRepository(db *sql.DB) repository.TeamRepository {
	return &teamRepository{db: db}
}

func (r *teamRepository) List(ctx context.Context, filter model.ListTeamsFilter) ([]*model.Team, error) {
	query := `SELECT id, name, code, description, team_type, status
	          FROM risk_team WHERE status = 'ACTIVE'`
	args := []any{}

	switch filter.Type {
	case "SOURCE_REGISTER":
		query += " AND team_type IN ('SOURCE_REGISTER', 'BOTH')"
	case "ASSIGNMENT":
		query += " AND team_type IN ('ASSIGNMENT', 'BOTH')"
	}
	query += " ORDER BY name"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []*model.Team
	for rows.Next() {
		t := &model.Team{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Code, &t.Description, &t.TeamType, &t.Status); err != nil {
			return nil, fmt.Errorf("scan team row: %w", err)
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

func (r *teamRepository) Create(ctx context.Context, req model.CreateTeamRequest, createdBy string) (*model.Team, error) {
	// TODO: implement risk_team INSERT
	return nil, nil
}

func (r *teamRepository) Update(ctx context.Context, id int, req model.UpdateTeamRequest, updatedBy string) error {
	// TODO: implement risk_team UPDATE
	return nil
}

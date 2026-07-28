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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type teamRepository struct{ c *entityclient.Client }

// NewTeamRepository creates a Compliance Entity-backed repository.TeamRepository.
func NewTeamRepository(c *entityclient.Client) repository.TeamRepository {
	return &teamRepository{c: c}
}

// entTeam is the entity's camelCase representation of a risk team. The entity
// also returns createdOn/updatedOn, which model.Team does not carry and which
// are deliberately dropped here.
type entTeam struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
	TeamType    string  `json:"teamType"`
	Status      string  `json:"status"`
}

type searchTeamsResponse struct {
	Teams []entTeam `json:"teams"`
	Total int       `json:"total"`
}

// List mirrors the MySQL implementation: ACTIVE teams only, filtered by the
// semantic Type value, ordered by name. The entity applies the same ORDER BY
// name, so ordering is preserved. The entity requires pagination while the
// MySQL query returned every match, so this pages until a short page.
func (r *teamRepository) List(ctx context.Context, filter model.ListTeamsFilter) ([]*model.Team, error) {
	// Semantic filter values expand to the same team_type sets the MySQL
	// implementation used; an empty Type sends no team-type filter at all.
	var teamTypeKeys []string
	switch filter.Type {
	case "SOURCE_REGISTER":
		teamTypeKeys = []string{"SOURCE_REGISTER", "BOTH"}
	case "ASSIGNMENT":
		teamTypeKeys = []string{"ASSIGNMENT", "BOTH"}
	}

	var teams []*model.Team
	for offset := 0; ; offset += pageLimit {
		body := map[string]any{
			"teamTypeKeys": teamTypeKeys,
			"statusKey":    "ACTIVE",
			"pagination":   map[string]int{"limit": pageLimit, "offset": offset},
		}
		var resp searchTeamsResponse
		if err := r.c.Post(ctx, "/risk/teams/search", body, &resp); err != nil {
			return nil, fmt.Errorf("list teams: %w", err)
		}
		for _, t := range resp.Teams {
			teams = append(teams, &model.Team{
				ID:          t.ID,
				Name:        t.Name,
				Code:        t.Code,
				Description: t.Description,
				TeamType:    t.TeamType,
				Status:      t.Status,
			})
		}
		if len(resp.Teams) < pageLimit {
			return teams, nil
		}
	}
}

// Create is not implemented, matching the MySQL implementation. The entity
// exposes POST /risk/teams, but no route reaches this method today and
// implementing it here would be a behaviour change, not a migration.
func (r *teamRepository) Create(ctx context.Context, req model.CreateTeamRequest, createdBy string) (*model.Team, error) {
	return nil, errNotImplemented
}

// Update is not implemented, matching the MySQL implementation. See Create.
func (r *teamRepository) Update(ctx context.Context, id int, req model.UpdateTeamRequest, updatedBy string) error {
	return errNotImplemented
}

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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type dashboardRepo struct{ c *entityclient.Client }

// NewDashboardRepository returns an entity-backed DashboardRepository.
func NewDashboardRepository(c *entityclient.Client) repository.DashboardRepository {
	return &dashboardRepo{c: c}
}

func (r *dashboardRepo) Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error) {
	// Scope is derived from the caller's privileges and sent explicitly:
	// viewScope drives stats/charts, workQueueScope the action lists.
	// The entity resolves the actor's team/owner/auditor identity from userID.
	body := map[string]any{
		"scope":          string(f.ViewScope),
		"workQueueScope": string(f.WorkQueueScope),
		"workQueueClass": string(f.WorkQueueClass),
		"userId":         f.UserID,
		"scopeTeamIds":   f.ScopeTeamIDs,
	}
	var data model.DashboardData
	if err := r.c.Post(ctx, "/audit/dashboard/search", body, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (r *dashboardRepo) GetWorkQueuePage(ctx context.Context, f model.DashboardFilter, tab model.WorkQueueTab, page, limit int) (*model.WorkQueuePage, error) {
	body := map[string]any{
		"workQueueScope": string(f.WorkQueueScope),
		"workQueueClass": string(f.WorkQueueClass),
		"userId":         f.UserID,
		"tab":            string(tab),
		"page":           page,
		"limit":          limit,
		"scopeTeamIds":   f.ScopeTeamIDs,
		"teamIds":        f.TeamIDs,
		"ownerIds":       f.OwnerIDs,
		"auditIds":       f.AuditIDs,
		"controlNumber":  f.ControlNumber,
		"statuses":       f.Statuses,
		"dueSortDesc":    f.DueSortDesc,
	}
	var p model.WorkQueuePage
	if err := r.c.Post(ctx, "/audit/work-queue/search", body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

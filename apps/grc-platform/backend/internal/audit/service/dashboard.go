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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
)

// DashboardService fetches role-scoped dashboard data.
type DashboardService interface {
	Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error)
	GetWorkQueuePage(ctx context.Context, f model.DashboardFilter, tab model.WorkQueueTab, page, limit int) (*model.WorkQueuePage, error)
}

type dashboardService struct {
	repo repository.DashboardRepository
	// directory resolves ProcessOwnerUUID/ProcessOwnerUserType to a display
	// name (see enrichProcessOwners) — the `user` table stores none itself.
	// Nil is tolerated (local dev without SCIM configured): names stay unset.
	directory *directory.Service
}

// NewDashboardService creates a DashboardService backed by repo.
func NewDashboardService(repo repository.DashboardRepository, dir *directory.Service) DashboardService {
	return &dashboardService{repo: repo, directory: dir}
}

func (s *dashboardService) Get(ctx context.Context, f model.DashboardFilter) (*model.DashboardData, error) {
	data, err := s.repo.Get(ctx, f)
	if err != nil {
		return nil, err
	}
	s.enrichProcessOwners(ctx, data.ActionItems, data.DueSoonItems, data.OverdueControls)
	return data, nil
}

func (s *dashboardService) GetWorkQueuePage(ctx context.Context, f model.DashboardFilter, tab model.WorkQueueTab, page, limit int) (*model.WorkQueuePage, error) {
	p, err := s.repo.GetWorkQueuePage(ctx, f, tab, page, limit)
	if err != nil {
		return nil, err
	}
	s.enrichProcessOwners(ctx, p.Items, nil, nil)
	return p, nil
}

// enrichProcessOwners batch-resolves every item's ProcessOwnerUUID to a
// display name via the identity directory — one batched lookup across all
// three lists together, not one per item or per list. Best-effort: a uuid the
// directory doesn't know (or an item with no owner at all) simply leaves
// ProcessOwner as "".
func (s *dashboardService) enrichProcessOwners(ctx context.Context, actionItems, dueSoonItems []model.ActionItem, overdue []model.OverdueControl) {
	if s.directory == nil {
		return
	}
	uuidTypes := map[string]string{}
	for _, it := range actionItems {
		if it.ProcessOwnerUUID != "" {
			uuidTypes[it.ProcessOwnerUUID] = it.ProcessOwnerUserType
		}
	}
	for _, it := range dueSoonItems {
		if it.ProcessOwnerUUID != "" {
			uuidTypes[it.ProcessOwnerUUID] = it.ProcessOwnerUserType
		}
	}
	for _, it := range overdue {
		if it.ProcessOwnerUUID != "" {
			uuidTypes[it.ProcessOwnerUUID] = it.ProcessOwnerUserType
		}
	}
	if len(uuidTypes) == 0 {
		return
	}
	people := s.directory.LookupAllTyped(ctx, uuidTypes)
	for i, it := range actionItems {
		if p, ok := people[it.ProcessOwnerUUID]; ok {
			actionItems[i].ProcessOwner = p.DisplayName
		}
	}
	for i, it := range dueSoonItems {
		if p, ok := people[it.ProcessOwnerUUID]; ok {
			dueSoonItems[i].ProcessOwner = p.DisplayName
		}
	}
	for i, it := range overdue {
		if p, ok := people[it.ProcessOwnerUUID]; ok {
			overdue[i].ProcessOwner = p.DisplayName
		}
	}
}

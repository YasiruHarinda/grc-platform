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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// AnalyticsService assembles the risk analytics summary payload.
type AnalyticsService interface {
	// Summary builds the analytics payload, optionally scoped to one
	// register (nil = all registers).
	Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error)
}

// assembledAnalyticsService serves an analytics payload that arrives already
// assembled.
//
// The Compliance Entity owns the trailing-window definition and the month
// scaffolding as well as the fourteen aggregate queries, so there is nothing
// left to compute here. See assembledDashboardService.
type assembledAnalyticsService struct {
	source interface {
		Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error)
	}
}

// NewAssembledAnalyticsService creates an AnalyticsService that passes an
// already-assembled payload straight through.
func NewAssembledAnalyticsService(source interface {
	Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error)
}) AnalyticsService {
	return &assembledAnalyticsService{source: source}
}

func (s *assembledAnalyticsService) Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error) {
	return s.source.Summary(ctx, registerID)
}

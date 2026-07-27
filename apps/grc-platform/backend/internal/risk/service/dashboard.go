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

// DashboardService assembles the risk dashboard payload.
type DashboardService interface {
	// Summary builds the dashboard payload, optionally scoped to one
	// register (nil = all registers).
	Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error)
}

// assembledDashboardService serves a dashboard that arrives already assembled.
//
// The Compliance Entity runs the aggregate queries and pivots the resulting
// facts into chart shapes, so nothing is left to do here. This used to be ~200
// lines of pivoting over seven fact-level repository calls; that logic, and the
// tests covering it, moved to the entity alongside the queries they depend on.
// The audit module's dashboard service has the same shape for the same reason.
type assembledDashboardService struct {
	source interface {
		Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error)
	}
}

// NewAssembledDashboardService creates a DashboardService that passes an
// already-assembled payload straight through.
func NewAssembledDashboardService(source interface {
	Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error)
}) DashboardService {
	return &assembledDashboardService{source: source}
}

func (s *assembledDashboardService) Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error) {
	return s.source.Summary(ctx, registerID)
}

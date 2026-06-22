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

// Package repository defines the data-access contracts for the Risk Hub module.
// Add methods to each interface as handlers are implemented.
package repository

import (
	"context"

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
)

// RiskRepository is the data-access contract for risk records.
// TODO: extend based on RISK_MODULE_DESIGN.md
type RiskRepository interface {
	List(ctx context.Context, filter model.ListRisksFilter) ([]*model.Risk, error)
	GetByID(ctx context.Context, id int) (*model.Risk, error)
	Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.Risk, error)
	Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error
	UpdateStatus(ctx context.Context, id int, status, updatedBy string) error
}

// TeamRepository is the data-access contract for risk teams.
// TODO: add CRUD methods
type TeamRepository interface{}

// RiskScoreRepository is the data-access contract for risk score configurations.
// TODO: add CRUD methods
type RiskScoreRepository interface{}

// ActionPlanRepository is the data-access contract for action plans and steps.
// TODO: add CRUD methods
type ActionPlanRepository interface{}

// RiskEvidenceRepository is the data-access contract for risk evidence files.
// TODO: add CRUD methods
type RiskEvidenceRepository interface{}

// EscalationRepository is the data-access contract for risk escalations.
// TODO: add CRUD methods
type EscalationRepository interface{}

// ChangelogRepository is the data-access contract for the risk audit trail.
// TODO: add insert and list methods
type ChangelogRepository interface{}

// NotificationRepository is the data-access contract for risk notifications.
// TODO: add methods (list, mark-read)
type NotificationRepository interface{}

// ComplianceReferenceRepository is the data-access contract for compliance references.
// TODO: add CRUD methods
type ComplianceReferenceRepository interface{}

// AnalyticsRepository provides aggregated read queries for the analytics summary endpoint.
// TODO: add Summary / count-by-status / count-by-level methods
type AnalyticsRepository interface{}

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
package repository

import (
	"context"

	"github.com/wso2-open-operations/grc-platform/backend/internal/risk/model"
)

// RiskRepository is the data-access contract for risk records.
type RiskRepository interface {
	List(ctx context.Context, filter model.ListRisksFilter) ([]*model.Risk, error)
	GetByID(ctx context.Context, id int) (*model.Risk, error)
	Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error)
	Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error
	UpdateStatus(ctx context.Context, id int, status, updatedBy string) error
	NextSequenceID(ctx context.Context, sourceRegisterID, year int, quarter string) (int, error)
}

// TeamRepository is the data-access contract for risk teams.
type TeamRepository interface {
	List(ctx context.Context, filter model.ListTeamsFilter) ([]*model.Team, error)
	Create(ctx context.Context, req model.CreateTeamRequest, createdBy string) (*model.Team, error)
	Update(ctx context.Context, id int, req model.UpdateTeamRequest, updatedBy string) error
}

// RiskScoreRepository is the data-access contract for risk score configurations.
type RiskScoreRepository interface {
	List(ctx context.Context) ([]*model.RiskScore, error)
	Create(ctx context.Context, req model.CreateRiskScoreRequest, createdBy string) (*model.RiskScore, error)
	Update(ctx context.Context, id int, req model.UpdateRiskScoreRequest, updatedBy string) error
}

// ActionPlanRepository is the data-access contract for action plans and steps.
type ActionPlanRepository interface {
	List(ctx context.Context, riskID int) ([]*model.ActionPlan, error)
	GetByID(ctx context.Context, planID int) (*model.ActionPlan, error)
	Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error)
	Update(ctx context.Context, planID int, req model.UpdateActionPlanRequest, updatedBy string) error
	ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error)
	AddStep(ctx context.Context, planID, stepNo int, req model.AddActionPlanStepRequest, createdBy string) (*model.ActionPlanStep, error)
	UpdateStep(ctx context.Context, stepID int, req model.UpdateActionPlanStepRequest, updatedBy string) error
}

// RiskEvidenceRepository is the data-access contract for risk evidence files.
type RiskEvidenceRepository interface {
	List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error)
	Create(ctx context.Context, riskID int, fileName, filePath, note, evidenceType, createdBy string) (*model.RiskEvidence, error)
	Delete(ctx context.Context, evidenceID int) error
}

// EscalationRepository is the data-access contract for risk escalations.
type EscalationRepository interface {
	List(ctx context.Context, riskID int) ([]*model.Escalation, error)
}

// ChangelogRepository is the data-access contract for the risk audit trail.
type ChangelogRepository interface{}

// NotificationRepository is the data-access contract for risk notifications.
type NotificationRepository interface {
	List(ctx context.Context, recipientID int) ([]*model.Notification, error)
	MarkRead(ctx context.Context, id, recipientID int) error
}

// ComplianceReferenceRepository is the data-access contract for compliance references.
type ComplianceReferenceRepository interface {
	List(ctx context.Context) ([]*model.ComplianceReference, error)
	Create(ctx context.Context, req model.CreateComplianceRefRequest, createdBy string) (*model.ComplianceReference, error)
}

// AnalyticsRepository provides aggregated read queries for the analytics summary endpoint.
type AnalyticsRepository interface {
	// TODO: add Summary / count-by-status / count-by-level methods
}

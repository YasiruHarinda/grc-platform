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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// RiskRepository is the data-access contract for risk records.
type RiskRepository interface {
	List(ctx context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error)
	GetByID(ctx context.Context, id int) (*model.RiskDetail, error)
	// GetWorkflowStatus is a lightweight single-column fetch for callers that only
	// need to guard a workflow transition, avoiding GetByID's full join + related-entity queries.
	GetWorkflowStatus(ctx context.Context, id int) (string, error)
	Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error)
	Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error
	// TransitionStatus atomically moves a risk from fromStatus to toStatus using a
	// conditional UPDATE (WHERE workflow_status = fromStatus). Returns 409 when 0 rows
	// are affected, meaning another request already changed the status concurrently.
	TransitionStatus(ctx context.Context, id int, fromStatus, toStatus, updatedBy string) error
	// RejectTransition atomically writes the rejection comment and stage and moves the
	// status to PENDING_REVISION in a single UPDATE (inherently atomic, no transaction needed).
	RejectTransition(ctx context.Context, id int, comment, stage, fromStatus, updatedBy string) error
	// ResubmitTransition atomically clears rejection info and advances the status from
	// PENDING_REVISION to toStatus in a single UPDATE.
	ResubmitTransition(ctx context.Context, id int, fromStatus, toStatus, updatedBy string) error
	SetRiskType(ctx context.Context, id int, riskType, updatedBy string) error
	SetOwnerFirstApprovedAt(ctx context.Context, id int, updatedBy string) error
	NextSequenceID(ctx context.Context, sourceRegisterID int) (int, error)
}

// RiskAssessmentRepository is the data-access contract for residual risk assessments.
type RiskAssessmentRepository interface {
	Create(ctx context.Context, riskID int, req model.CreateAssessmentRequest, assessedBy string) (*model.RiskAssessment, error)
	ListByRiskID(ctx context.Context, riskID int) ([]model.RiskAssessment, error)
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

// AnalyticsRepository provides the aggregated read queries behind
// GET /api/v1/risks/analytics/summary. Every method accepts an optional
// registerID (nil = all registers) to scope its result to the page's
// register filter. All methods exclude CANCELLED risks.
type AnalyticsRepository interface {
	// NewThisMonthCount returns the count of risks identified since monthStart.
	NewThisMonthCount(ctx context.Context, registerID *int, monthStart string) (int, error)
	// AvgDaysToClose returns the average identified→closed duration in days
	// across CLOSED risks, or nil if there are none.
	AvgDaysToClose(ctx context.Context, registerID *int) (*float64, error)
	// AvgEffectiveScore returns the average effective residual score across
	// open risks, or nil if there are none.
	AvgEffectiveScore(ctx context.Context, registerID *int) (*float64, error)
	// IdentifiedTrend returns, per month since `since`, the count of risks
	// identified and their average effective score.
	IdentifiedTrend(ctx context.Context, registerID *int, since string) ([]model.MonthScoreStat, error)
	// ClosedTrend returns, per month since `since`, the count of risks closed
	// (approximated by workflow_status=CLOSED and updated_at).
	ClosedTrend(ctx context.Context, registerID *int, since string) ([]model.MonthCount, error)
	// LevelDistribution returns, per month since `since` × effective level,
	// the count of risks identified that month.
	LevelDistribution(ctx context.Context, registerID *int, since string) ([]model.MonthLevelCount, error)
	// LevelReference returns every distinct risk level defined in risk_score,
	// ordered by severity (highest first), with its reference color.
	LevelReference(ctx context.Context) ([]model.RiskLevelRef, error)
	// IdentifiedTrendByRegister returns, per month since `since` × register,
	// the count of risks identified that month.
	IdentifiedTrendByRegister(ctx context.Context, registerID *int, since string) ([]model.MonthRegisterCount, error)
	// ClosedTrendByRegister returns, per month since `since` × register, the
	// count of risks closed that month.
	ClosedTrendByRegister(ctx context.Context, registerID *int, since string) ([]model.MonthRegisterCount, error)
	// RegisterTotals returns total risk count (all-time, all statuses except
	// CANCELLED) per register, for the cross-register comparison donut.
	RegisterTotals(ctx context.Context) ([]model.RegisterShare, error)
	// ComplianceDistribution returns total risk count per compliance
	// framework, all-time.
	ComplianceDistribution(ctx context.Context, registerID *int) ([]model.ComplianceShare, error)
	// TreatmentMix returns open risk count per treatment strategy.
	TreatmentMix(ctx context.Context, registerID *int) ([]model.TreatmentShare, error)
	// WorkflowFunnel returns risk count per workflow_status stage.
	WorkflowFunnel(ctx context.Context, registerID *int) ([]model.WorkflowStageCount, error)
	// AgingRisks returns open risks ordered oldest-identified-first.
	AgingRisks(ctx context.Context, registerID *int, limit int) ([]model.AgingRiskItem, error)
}

// DashboardRepository provides the aggregated read queries behind GET /api/v1/risks/dashboard.
// All methods exclude CANCELLED risks; "open" means any status other than CLOSED.
type DashboardRepository interface {
	// StatusCounts returns total / open / closed risk counts.
	StatusCounts(ctx context.Context) (*model.RiskStatusSummary, error)
	// OpenRiskFacts returns open risks grouped by register × effective residual
	// score cell × treatment strategy; the service derives all open-risk charts from it.
	OpenRiskFacts(ctx context.Context) ([]model.OpenRiskFact, error)
	// CertTagCounts returns open cert-tag occurrences per register × certification.
	CertTagCounts(ctx context.Context) ([]model.RegisterCertCount, error)
	// RepeatedComplianceRisks returns per-register occurrences of cert-tagged risk
	// titles that appear in two or more source registers.
	RepeatedComplianceRisks(ctx context.Context) ([]model.RepeatedRiskRow, error)
	// HighRisks returns open risks whose effective residual level is HIGH,
	// oldest identified first.
	HighRisks(ctx context.Context) ([]model.HighRiskItem, error)
}

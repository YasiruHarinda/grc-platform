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
//
// Every contract here is implemented against the Compliance Entity; the module
// holds no database handle. The dashboard and analytics payloads have no
// contract here at all — the entity assembles them and the services pass them
// through, so there is nothing for this package to describe.
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
// Read-only: the score matrix is reference data, seeded and edited out of band.
// Create and Update were declared here but never routed and never implemented —
// the service stubs returned nil without calling them — so they were dropped
// rather than migrated. The Compliance Entity likewise exposes only a read.
type RiskScoreRepository interface {
	List(ctx context.Context) ([]*model.RiskScore, error)
}

// ActionPlanRepository is the data-access contract for action plans and steps.
type ActionPlanRepository interface {
	List(ctx context.Context, riskID int) ([]*model.ActionPlan, error)
	GetByID(ctx context.Context, planID int) (*model.ActionPlan, error)
	Create(ctx context.Context, riskID int, req model.CreateActionPlanRequest, createdBy string) (*model.ActionPlan, error)
	ListSteps(ctx context.Context, planID int) ([]*model.ActionPlanStep, error)
	UpdateStep(ctx context.Context, planID, stepID int, req model.UpdateActionPlanStepRequest, updatedBy string) error
	// Complete marks a plan COMPLETED once every step is done. The entity
	// notifies the risk assigner as part of the same cascade; it no longer
	// resolves an escalation or reverts the risk, which an escalation comment
	// does instead.
	Complete(ctx context.Context, planID int, updatedBy string) (*model.ActionPlan, error)
}

// RiskEvidenceRepository is the data-access contract for risk evidence files.
type RiskEvidenceRepository interface {
	List(ctx context.Context, riskID int) ([]*model.RiskEvidence, error)
	Create(ctx context.Context, riskID int, fileName, filePath, note, evidenceType, createdBy string) (*model.RiskEvidence, error)
	Delete(ctx context.Context, riskID, evidenceID int, byUserID string) error
}

// EscalationRepository is the data-access contract for risk escalations.
type EscalationRepository interface {
	List(ctx context.Context, riskID int) ([]*model.Escalation, error)
	// Escalate is the manual trigger — Compliance/Admin escalating an overdue
	// IN_REMEDIATION risk on demand instead of waiting for the daily job.
	// Escalate takes the assigner's and action owner's line-manager emails,
	// already resolved by the caller — the entity has no HR client of its own.
	Escalate(ctx context.Context, riskID int, createdBy string, assignerLead, actionOwnerLead *string) (*model.Escalation, error)
	// Comment records the management/lead comment on an escalation. Leaving the
	// escalation OPEN is deliberate: it is what keeps the risk in the Overdue
	// tab until the assigner submits for completion approval.
	Comment(ctx context.Context, riskID, escalationID int, comment, updatedBy string) (*model.Escalation, error)
	// Resolve closes every open escalation on a risk. Called when the assigner
	// submits for completion approval, which is what drops the risk out of the
	// Overdue tab.
	Resolve(ctx context.Context, riskID int, updatedBy string) error
}

// HistoryRepository is the data-access contract for a risk's history — the
// risk_change_log table, which holds both field diffs and workflow events.
type HistoryRepository interface {
	// List returns a risk's history newest-first.
	List(ctx context.Context, riskID int) ([]*model.HistoryEntry, error)
	// Record appends one entry. Callers treat failures as non-fatal: a missing
	// history row must never fail the action it was recording.
	Record(ctx context.Context, riskID int, req model.RecordHistoryRequest, createdBy string) error
}

// NotificationRepository is the data-access contract for risk notifications.
type NotificationRepository interface {
	List(ctx context.Context, recipientID int) ([]*model.Notification, error)
	MarkRead(ctx context.Context, id, recipientID int) error
}

// ComplianceReferenceRepository is the data-access contract for compliance references.
// Read-only, for the same reason as RiskScoreRepository: Create was declared but
// never routed and never implemented — the service stub returned nil without
// calling it — so it was dropped rather than migrated.
type ComplianceReferenceRepository interface {
	List(ctx context.Context) ([]*model.ComplianceReference, error)
}

// RiskCategoryRepository is the data-access contract for risk categories.
type RiskCategoryRepository interface {
	List(ctx context.Context) ([]*model.RiskCategory, error)
}

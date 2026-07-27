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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

// Package service contains business logic between HTTP handlers and the repository layer.
package service

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// UserService defines operations on the user entity.
type UserService interface {
	SearchUsers(ctx context.Context, req domain.SearchUsersRequest) (domain.SearchUsersResponse, error)
	GetUserByID(ctx context.Context, id int) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	CreateUser(ctx context.Context, req domain.CreateUserRequest) (domain.User, error)
	UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (domain.User, error)
}

// AuditTeamService defines operations on the audit_team entity.
type AuditTeamService interface {
	SearchAuditTeams(ctx context.Context, req domain.SearchAuditTeamsRequest) (domain.SearchAuditTeamsResponse, error)
	GetAuditTeamByID(ctx context.Context, id int) (domain.AuditTeam, error)
	CreateAuditTeam(ctx context.Context, req domain.CreateAuditTeamRequest) (domain.AuditTeam, error)
	UpdateAuditTeam(ctx context.Context, id int, req domain.UpdateAuditTeamRequest) (domain.AuditTeam, error)
}

// FrameworkControlService defines operations on the audit_framework_control table.
// Rows are immutable once created — updates produce a new version row.
type FrameworkControlService interface {
	ListCurrentControls(ctx context.Context, frameworkID int) (domain.ListFrameworkControlsResponse, error)
	ListAllVersions(ctx context.Context, frameworkID int, controlNumber string) ([]domain.AuditFrameworkControl, error)
	GetByID(ctx context.Context, id int) (domain.AuditFrameworkControl, error)
	Create(ctx context.Context, frameworkID int, req domain.CreateFrameworkControlRequest) (domain.AuditFrameworkControl, error)
	NewVersion(ctx context.Context, id int, req domain.UpdateFrameworkControlRequest) (domain.AuditFrameworkControl, error)
}

// AuditFrameworkService defines operations on the audit_framework entity.
type AuditFrameworkService interface {
	SearchAuditFrameworks(ctx context.Context, req domain.SearchAuditFrameworksRequest) (domain.SearchAuditFrameworksResponse, error)
	GetAuditFrameworkByID(ctx context.Context, id int) (domain.AuditFramework, error)
	CreateAuditFramework(ctx context.Context, req domain.CreateAuditFrameworkRequest) (domain.AuditFramework, error)
	UpdateAuditFramework(ctx context.Context, id int, req domain.UpdateAuditFrameworkRequest) (domain.AuditFramework, error)
}

// AuditProductService defines operations on the audit_product entity.
type AuditProductService interface {
	SearchAuditProducts(ctx context.Context, req domain.SearchAuditProductsRequest) (domain.SearchAuditProductsResponse, error)
	GetAuditProductByID(ctx context.Context, id int) (domain.AuditProduct, error)
	CreateAuditProduct(ctx context.Context, req domain.CreateAuditProductRequest) (domain.AuditProduct, error)
	UpdateAuditProduct(ctx context.Context, id int, req domain.UpdateAuditProductRequest) (domain.AuditProduct, error)
}

// AuditService defines operations on the audit entity.
type AuditService interface {
	SearchAudits(ctx context.Context, req domain.SearchAuditsRequest) (domain.SearchAuditsResponse, error)
	GetAuditByID(ctx context.Context, id int) (domain.Audit, error)
	CreateAudit(ctx context.Context, req domain.CreateAuditRequest) (domain.Audit, error)
	UpdateAudit(ctx context.Context, id int, req domain.UpdateAuditRequest) (domain.Audit, error)
	DeleteAudit(ctx context.Context, id int, deletedBy string) error
}

// ControlService defines operations on the audit_control entity.
type ControlService interface {
	SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) (domain.SearchControlsResponse, error)
	SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) (domain.SearchControlsResponse, error)
	GetControlByID(ctx context.Context, auditID, controlID int) (domain.AuditControl, error)
	CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (domain.AuditControl, error)
	BulkCreateControls(ctx context.Context, auditID int, req domain.BulkCreateControlsRequest) (domain.BulkCreateControlsResponse, error)
	UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (domain.AuditControl, error)
	DeleteControl(ctx context.Context, auditID, controlID int) error
	ListAssignedForEvidence(ctx context.Context, userEmail string) (domain.ListAssignedControlsResponse, error)
	// GetEvidenceAssignment confirms userEmail is assigned to an actionable control
	// and returns its audit id (for server-side folder-path derivation).
	GetEvidenceAssignment(ctx context.Context, userEmail string, controlID int) (domain.EvidenceAssignmentResponse, error)
	// FindActivePopulation returns the active population round for an OE control.
	FindActivePopulation(ctx context.Context, controlID int) (domain.ActivePopulationResponse, error)
}

// EvidenceService defines operations on audit_evidence and audit_evidence_file.
type EvidenceService interface {
	CreateEvidence(ctx context.Context, controlID int, req domain.CreateEvidenceRequest) (domain.AuditEvidence, error)
	GetEvidenceByID(ctx context.Context, evidenceID int) (domain.AuditEvidence, error)
	ListEvidenceByControl(ctx context.Context, auditID, controlID int) (domain.ListEvidenceResponse, error)
	UpdateEvidence(ctx context.Context, evidenceID int, req domain.UpdateEvidenceRequest) (domain.AuditEvidence, error)
	AddEvidenceFile(ctx context.Context, evidenceID int, req domain.CreateEvidenceFileRequest) (domain.AuditEvidenceFile, error)
	ListEvidenceFiles(ctx context.Context, evidenceID int) (domain.ListEvidenceFilesResponse, error)
	GetEvidenceFileByID(ctx context.Context, fileID int) (domain.AuditEvidenceFile, error)
	DeleteEvidenceFile(ctx context.Context, fileID int) error
}

// PopulationService defines operations on audit_population and its files.
type PopulationService interface {
	CreatePopulation(ctx context.Context, auditID, controlID int, req domain.CreatePopulationRequest) (domain.AuditPopulation, error)
	GetPopulationByID(ctx context.Context, populationID int) (domain.AuditPopulation, error)
	ListPopulations(ctx context.Context, auditID, controlID int) ([]domain.AuditPopulation, error)
	UpdatePopulation(ctx context.Context, populationID int, req domain.UpdatePopulationRequest) (domain.AuditPopulation, error)
	AddPopulationFile(ctx context.Context, populationID int, req domain.CreatePopulationFileRequest) (domain.AuditEvidenceFile, error)
	ListPopulationFiles(ctx context.Context, populationID int) ([]domain.AuditEvidenceFile, error)
	DeletePopulationFile(ctx context.Context, fileID int) error
}

// RiskTeamService defines operations on the risk_team entity.
type RiskTeamService interface {
	SearchRiskTeams(ctx context.Context, req domain.SearchRiskTeamsRequest) (domain.SearchRiskTeamsResponse, error)
	GetRiskTeamByID(ctx context.Context, id int) (domain.RiskTeam, error)
	CreateRiskTeam(ctx context.Context, req domain.CreateRiskTeamRequest) (domain.RiskTeam, error)
	UpdateRiskTeam(ctx context.Context, id int, req domain.UpdateRiskTeamRequest) (domain.RiskTeam, error)
}

// RiskScoreService defines operations on the risk_score entity.
type RiskScoreService interface {
	ListRiskScores(ctx context.Context) (domain.ListRiskScoresResponse, error)
}

// RiskReferenceService defines operations on risk_security_compliance_reference.
type RiskReferenceService interface {
	SearchRiskReferences(ctx context.Context, req domain.SearchRiskReferencesRequest) (domain.SearchRiskReferencesResponse, error)
	GetRiskReferenceByID(ctx context.Context, id int) (domain.RiskComplianceReference, error)
	CreateRiskReference(ctx context.Context, req domain.CreateRiskReferenceRequest) (domain.RiskComplianceReference, error)
	UpdateRiskReference(ctx context.Context, id int, req domain.UpdateRiskReferenceRequest) (domain.RiskComplianceReference, error)
}

// RiskService defines operations on the risk entity.
type RiskService interface {
	SearchRisks(ctx context.Context, req domain.SearchRisksRequest) (domain.SearchRisksResponse, error)
	GetRiskByID(ctx context.Context, id int) (domain.Risk, error)
	CreateRisk(ctx context.Context, req domain.CreateRiskRequest) (domain.Risk, error)
	UpdateRisk(ctx context.Context, id int, req domain.UpdateRiskRequest) (domain.Risk, error)
	NextSequenceNumber(ctx context.Context, sourceRegisterID int) (domain.NextSequenceResponse, error)
	GetRiskDetail(ctx context.Context, id int) (domain.RiskDetail, error)
}

// RiskActionPlanService defines operations on risk_action_plan.
type RiskActionPlanService interface {
	CreateRiskActionPlan(ctx context.Context, riskID int, req domain.CreateRiskActionPlanRequest) (domain.RiskActionPlan, error)
	GetRiskActionPlanByID(ctx context.Context, planID int) (domain.RiskActionPlan, error)
	UpdateRiskActionPlan(ctx context.Context, planID int, req domain.UpdateRiskActionPlanRequest) (domain.RiskActionPlan, error)
	ListRiskActionPlans(ctx context.Context, riskID int) (domain.ListRiskActionPlansResponse, error)
	// CompleteRiskActionPlan marks a plan COMPLETED once every step is done.
	// For a MANAGEMENT plan this also resolves the linked escalation and
	// reverts the risk ESCALATED -> IN_REMEDIATION.
	CompleteRiskActionPlan(ctx context.Context, planID int, req domain.CompleteRiskActionPlanRequest) (domain.RiskActionPlan, error)
}

// RiskEvidenceService defines operations on risk_evidence_file.
type RiskEvidenceService interface {
	CreateRiskEvidence(ctx context.Context, riskID int, req domain.CreateRiskEvidenceRequest) (domain.RiskEvidenceFile, error)
	ListRiskEvidence(ctx context.Context, riskID int) (domain.ListRiskEvidenceResponse, error)
	DeleteRiskEvidence(ctx context.Context, fileID int) error
}

// RiskAssessmentService defines operations on risk_assessment.
type RiskAssessmentService interface {
	CreateRiskAssessment(ctx context.Context, riskID int, req domain.CreateRiskAssessmentRequest) (domain.RiskAssessment, error)
	ListRiskAssessments(ctx context.Context, riskID int) (domain.ListRiskAssessmentsResponse, error)
}

// TrailService defines operations on audit_trail.
type TrailService interface {
	CreateTrail(ctx context.Context, auditID int, req domain.CreateAuditTrailRequest) (domain.AuditTrail, error)
	ListTrail(ctx context.Context, auditID int, limit, offset int) (domain.ListAuditTrailResponse, error)
}

// CommentService defines operations on audit_comment (evidence-scoped).
type CommentService interface {
	CreateComment(ctx context.Context, evidenceID int, req domain.CreateAuditCommentRequest) (domain.AuditComment, error)
	ListCommentsByEvidence(ctx context.Context, evidenceID int) (domain.ListAuditCommentsResponse, error)
	DeleteComment(ctx context.Context, commentID int) error
}

// AIValidationService defines operations on audit_ai_validation_log (append-only).
type AIValidationService interface {
	CreateValidation(ctx context.Context, evidenceID int, req domain.CreateAuditAIValidationLogRequest) (domain.AuditAIValidationLog, error)
	ListValidationsByEvidence(ctx context.Context, evidenceID int) (domain.ListAuditAIValidationLogsResponse, error)
}

// RiskActionStepService defines operations on risk_action_step.
type RiskActionStepService interface {
	CreateRiskActionStep(ctx context.Context, planID int, req domain.CreateRiskActionStepRequest) (domain.RiskActionStep, error)
	GetRiskActionStepByID(ctx context.Context, planID, stepID int) (domain.RiskActionStep, error)
	UpdateRiskActionStep(ctx context.Context, planID, stepID int, req domain.UpdateRiskActionStepRequest) (domain.RiskActionStep, error)
	DeleteRiskActionStep(ctx context.Context, planID, stepID int) error
	ListRiskActionSteps(ctx context.Context, planID int) (domain.ListRiskActionStepsResponse, error)
}

// RiskComplianceRefService defines operations on the risk_compliance_reference junction.
type RiskComplianceRefService interface {
	AddRiskComplianceRef(ctx context.Context, riskID int, req domain.AddRiskComplianceRefRequest) (domain.RiskComplianceRefLink, error)
	DeleteRiskComplianceRef(ctx context.Context, riskID, referenceID int) error
	ListRiskComplianceRefs(ctx context.Context, riskID int) (domain.ListRiskComplianceRefsResponse, error)
}

// RiskEscalationService defines operations on risk_escalation.
type RiskEscalationService interface {
	CreateRiskEscalation(ctx context.Context, riskID int, req domain.CreateRiskEscalationRequest) (domain.RiskEscalation, error)
	GetRiskEscalationByID(ctx context.Context, riskID, escalationID int) (domain.RiskEscalation, error)
	UpdateRiskEscalation(ctx context.Context, riskID, escalationID int, req domain.UpdateRiskEscalationRequest) (domain.RiskEscalation, error)
	ListRiskEscalations(ctx context.Context, riskID int) (domain.ListRiskEscalationsResponse, error)
	// EscalateRisk is the single "escalate one risk" operation shared by the
	// daily job (internal/job) and a manual trigger (Compliance clicking
	// Escalate on an overdue IN_REMEDIATION risk they've spotted before the
	// job gets to it): validates IN_REMEDIATION + overdue, creates the
	// OPEN escalation, and flips workflow_status -> ESCALATED. actorEmail is
	// "system" for the job, the caller's email for a manual trigger.
	EscalateRisk(ctx context.Context, riskID int, req domain.EscalateRiskRequest) (domain.RiskEscalation, error)
}

// RiskChangeLogService defines operations on risk_change_log (append-only).
type RiskChangeLogService interface {
	CreateRiskChangeLog(ctx context.Context, riskID int, req domain.CreateRiskChangeLogRequest) (domain.RiskChangeLog, error)
	ListRiskChangeLog(ctx context.Context, riskID int, limit, offset int) (domain.ListRiskChangeLogResponse, error)
}

// RiskNotificationService defines operations on risk_notification.
type RiskNotificationService interface {
	CreateRiskNotification(ctx context.Context, req domain.CreateRiskNotificationRequest) (domain.RiskNotification, error)
	ListRiskNotifications(ctx context.Context, recipientID int) (domain.ListRiskNotificationsResponse, error)
	MarkRiskNotificationRead(ctx context.Context, id int64, req domain.MarkRiskNotificationReadRequest) (domain.RiskNotification, error)
}

// DashboardService defines the read query for the audit dashboard.
type DashboardService interface {
	Get(ctx context.Context, req domain.AuditDashboardRequest) (*domain.DashboardData, error)
	GetWorkQueuePage(ctx context.Context, req domain.WorkQueueRequest) (*domain.WorkQueuePage, error)
}

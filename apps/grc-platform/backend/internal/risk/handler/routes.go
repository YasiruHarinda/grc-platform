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

// Package handler contains the HTTP handlers for the Risk Hub module.
package handler

import (
	"fmt"
	"net/http"

	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// Deps holds all service dependencies for Risk Hub handlers.
type Deps struct {
	Risk         riskservice.RiskService
	Assessment   riskservice.RiskAssessmentService
	Team         riskservice.TeamService
	Score        riskservice.RiskScoreService
	ActionPlan   riskservice.ActionPlanService
	Evidence     riskservice.EvidenceService
	Escalation   riskservice.EscalationService
	History      riskservice.HistoryService
	Notification riskservice.NotificationService
	Compliance   riskservice.ComplianceReferenceService
	Category     riskservice.RiskCategoryService
	Analytics    riskservice.AnalyticsService
	Dashboard    riskservice.DashboardService
	Employee     riskservice.EmployeeSearchService
	// Users resolves an authenticated caller's email to their internal
	// user.id — used by handleListRisks (Action Owner list scoping) and the
	// action-plan handlers (ownership checks).
	Users user.Repository
	// SCIM answers "which users belong to Asgardeo group X" for role-filtered
	// pickers (Management Approver, Risk Owner) — see internal/scim.
	SCIM *scim.Client
	// Email sends the risk-owner notification fired synchronously right
	// after a risk is created. A delivery failure is logged but never fails
	// risk creation itself — see handleCreateRisk.
	Email *emailer.Client
	// FrontendBaseURL is used to build the risk-detail link inside that
	// notification email.
	FrontendBaseURL string
}

// RegisterRoutes mounts all Risk Hub routes onto mux under /api/v1.
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	d := &deps

	// Teams
	mux.HandleFunc("GET /api/v1/teams", d.handleListTeams)

	// Risk scores
	mux.HandleFunc("GET /api/v1/risk-scores", d.handleListRiskScores)

	// Compliance references
	mux.HandleFunc("GET /api/v1/compliance-references", d.handleListComplianceReferences)

	// Risk categories
	mux.HandleFunc("GET /api/v1/risk-categories", d.handleListRiskCategories)

	// Role-filtered user pickers, sourced live from Asgardeo group membership
	// via the SCIM Operations Service (this platform keeps no user<->role
	// table of its own)
	mux.HandleFunc("GET /api/v1/management-approvers", d.handleListManagementApprovers)
	mux.HandleFunc("GET /api/v1/risk-owner-candidates", d.handleListRiskOwnerCandidates)

	// Current user
	mux.HandleFunc("GET /api/v1/me/privileges", d.handleGetMyPrivileges)

	// Employees (HR entity)
	mux.HandleFunc("GET /api/v1/employees/search", d.handleSearchEmployees)

	// Risks
	mux.HandleFunc("GET /api/v1/risks/next-sequence-id", d.handleNextSequenceID)
	mux.HandleFunc("GET /api/v1/risks", d.handleListRisks)
	mux.HandleFunc("POST /api/v1/risks", d.handleCreateRisk)
	mux.HandleFunc("GET /api/v1/risks/{id}", d.handleGetRisk)
	mux.HandleFunc("PUT /api/v1/risks/{id}", d.handleUpdateRisk)

	// Workflow transitions
	mux.HandleFunc("POST /api/v1/risks/{id}/owner-approve", d.handleOwnerApproveRisk)
	mux.HandleFunc("POST /api/v1/risks/{id}/management-approve", d.handleManagementApproveRisk)
	mux.HandleFunc("POST /api/v1/risks/{id}/approve", d.handleApproveRisk)
	mux.HandleFunc("POST /api/v1/risks/{id}/reject", d.handleRejectRisk)
	mux.HandleFunc("POST /api/v1/risks/{id}/complete", d.handleCompleteRisk)
	mux.HandleFunc("POST /api/v1/risks/{id}/resubmit", d.handleResubmitRisk)
	mux.HandleFunc("POST /api/v1/risks/{id}/close", d.handleCloseRisk)
	mux.HandleFunc("POST /api/v1/risks/{id}/cancel", d.handleCancelRisk)

	// Assessment
	mux.HandleFunc("POST /api/v1/risks/{id}/assess", d.handleAssessRisk)

	// Dashboard
	mux.HandleFunc("GET /api/v1/risks/dashboard", d.handleDashboard)

	// Analytics
	mux.HandleFunc("GET /api/v1/risks/analytics/summary", d.handleAnalyticsSummary)

	// Action plans (additional plans added by the Risk Assigner; step
	// completion by the plan's Action Owner)
	mux.HandleFunc("POST /api/v1/risks/{id}/action-plans", d.handleCreateActionPlan)
	mux.HandleFunc("GET /api/v1/risks/{id}/action-plans", d.handleListActionPlans)
	mux.HandleFunc("GET /api/v1/risks/{id}/action-plans/{planId}/steps", d.handleListActionPlanSteps)
	mux.HandleFunc("PATCH /api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}", d.handleUpdateActionPlanStep)
	mux.HandleFunc("POST /api/v1/risks/{id}/action-plans/{planId}/complete", d.handleCompleteActionPlan)

	// Escalations (automatic by default — see internal/risk/job — plus a manual
	// trigger for Compliance/Admin. Answered with a comment, which returns the
	// risk to its assigner; the escalation itself stays OPEN until the assigner
	// submits for completion approval)
	mux.HandleFunc("POST /api/v1/risks/{id}/escalate", d.handleEscalateRisk)
	mux.HandleFunc("GET /api/v1/risks/{id}/escalations", d.handleListEscalations)

	// Full risk history — every workflow event and field edit, behind the
	// drawer's History tab.
	mux.HandleFunc("GET /api/v1/risks/{id}/history", d.handleListRiskHistory)
	// Answering an escalation: a comment returns the risk to its assigner.
	// Replaces the MANAGEMENT action plan that used to serve this purpose.
	mux.HandleFunc("POST /api/v1/risks/{id}/escalations/{escalationId}/comment", d.handleEscalationComment)

	// TODO: remaining routes
	// GET/POST/DELETE /api/v1/risks/{id}/evidence
	// GET/PATCH /api/v1/notifications
	// POST/PUT /api/v1/teams
	// POST/PUT /api/v1/risk-scores
	// POST   /api/v1/compliance-references
}

// errorf is a convenience wrapper used by validation helpers.
func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

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

	riskservice "github.com/wso2-open-operations/grc-platform/backend/internal/risk/service"
)

// Deps holds all service dependencies for Risk Hub handlers.
type Deps struct {
	Risk         riskservice.RiskService
	Team         riskservice.TeamService
	Score        riskservice.RiskScoreService
	ActionPlan   riskservice.ActionPlanService
	Evidence     riskservice.EvidenceService
	Escalation   riskservice.EscalationService
	Notification riskservice.NotificationService
	Compliance   riskservice.ComplianceReferenceService
	Analytics    riskservice.AnalyticsService
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

	// Risks
	mux.HandleFunc("GET /api/v1/risks/next-sequence-id", d.handleNextSequenceID)
	mux.HandleFunc("POST /api/v1/risks", d.handleCreateRisk)

	// TODO: remaining risk routes
	// GET    /api/v1/risks
	// GET    /api/v1/risks/{id}
	// PUT    /api/v1/risks/{id}
	// POST   /api/v1/risks/{id}/submit
	// POST   /api/v1/risks/{id}/approve
	// POST   /api/v1/risks/{id}/reject
	// POST   /api/v1/risks/{id}/complete
	// POST   /api/v1/risks/{id}/owner-approve
	// POST   /api/v1/risks/{id}/close
	// POST   /api/v1/risks/{id}/escalate
	// POST   /api/v1/risks/{id}/assess
	// GET    /api/v1/risks/{id}/changelog
	// GET    /api/v1/risks/{id}/action-plans + POST
	// GET    /api/v1/risks/{id}/action-plans/{planId} + PUT
	// GET/POST/PUT /api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}
	// GET/POST/DELETE /api/v1/risks/{id}/evidence  ← needed for frontend evidence attachments (risk_evidence table, Azure Blob upload)
	// GET    /api/v1/risks/{id}/escalations
	// GET/PATCH /api/v1/notifications
	// GET    /api/v1/analytics/summary
	// POST/PUT /api/v1/teams
	// POST/PUT /api/v1/risk-scores
	// POST   /api/v1/compliance-references
}

// errorf is a convenience wrapper used by validation helpers.
func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

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
	"net/http"

	riskservice "github.com/wso2-open-operations/grc-platform/backend/internal/risk/service"
)

// Deps holds all service dependencies for Risk Hub handlers.
// Handlers call service methods; services call repositories.
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
// TODO: instantiate handler structs from deps and register each route below
//
// Routes to implement (from RISK_MODULE_DESIGN.md):
//
//	GET    /api/v1/users
//	GET    /api/v1/users/me
//	GET    /api/v1/teams
//	POST   /api/v1/teams
//	PUT    /api/v1/teams/{id}
//	GET    /api/v1/risk-scores
//	POST   /api/v1/risk-scores
//	PUT    /api/v1/risk-scores/{id}
//	GET    /api/v1/risks
//	POST   /api/v1/risks
//	GET    /api/v1/risks/{id}
//	PUT    /api/v1/risks/{id}
//	POST   /api/v1/risks/{id}/submit
//	POST   /api/v1/risks/{id}/approve
//	POST   /api/v1/risks/{id}/reject
//	POST   /api/v1/risks/{id}/complete
//	POST   /api/v1/risks/{id}/owner-approve
//	POST   /api/v1/risks/{id}/close
//	POST   /api/v1/risks/{id}/escalate
//	POST   /api/v1/risks/{id}/assess
//	GET    /api/v1/risks/{id}/changelog
//	GET    /api/v1/risks/{id}/action-plans
//	POST   /api/v1/risks/{id}/action-plans
//	GET    /api/v1/risks/{id}/action-plans/{planId}
//	PUT    /api/v1/risks/{id}/action-plans/{planId}
//	GET    /api/v1/risks/{id}/action-plans/{planId}/steps
//	POST   /api/v1/risks/{id}/action-plans/{planId}/steps
//	PUT    /api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}
//	GET    /api/v1/risks/{id}/evidence
//	POST   /api/v1/risks/{id}/evidence
//	DELETE /api/v1/risks/{id}/evidence/{evidenceId}
//	GET    /api/v1/risks/{id}/escalations
//	GET    /api/v1/notifications
//	PATCH  /api/v1/notifications/{id}/read
//	GET    /api/v1/compliance-references
//	POST   /api/v1/compliance-references
//	GET    /api/v1/analytics/summary
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	// TODO: register routes
}

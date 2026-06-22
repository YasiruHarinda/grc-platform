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

// Package handler contains the HTTP handlers for the Audit Hub module.
package handler

import (
	"net/http"

	auditservice "github.com/wso2-open-operations/grc-platform/backend/internal/audit/service"
)

// Deps holds all service dependencies for Audit Hub handlers.
// Handlers call service methods; services call repositories.
type Deps struct {
	Audit        auditservice.AuditService
	Control      auditservice.ControlService
	Evidence     auditservice.EvidenceService
	Population   auditservice.PopulationService
	Framework    auditservice.FrameworkService
	Comment      auditservice.CommentService
	Review       auditservice.ReviewService
	Notification auditservice.NotificationService
	Assignment   auditservice.AssignmentService
	Trail        auditservice.TrailService
}

// RegisterRoutes mounts all Audit Hub routes onto mux under /api/v1/audit.
// TODO: instantiate handler structs from deps and register each route below
//
// Routes to implement (from AUDIT_MODULE_DESIGN.md):
//
//	GET    /api/v1/audit/frameworks
//	POST   /api/v1/audit/frameworks
//	GET    /api/v1/audit/products
//	POST   /api/v1/audit/products
//	GET    /api/v1/audits
//	POST   /api/v1/audits
//	GET    /api/v1/audits/{id}
//	PUT    /api/v1/audits/{id}
//	POST   /api/v1/audits/{id}/fieldwork
//	POST   /api/v1/audits/{id}/review
//	POST   /api/v1/audits/{id}/complete
//	GET    /api/v1/audits/{id}/controls
//	POST   /api/v1/audits/{id}/controls
//	GET    /api/v1/audits/{id}/controls/{controlId}
//	PUT    /api/v1/audits/{id}/controls/{controlId}
//	GET    /api/v1/audits/{id}/controls/{controlId}/population
//	POST   /api/v1/audits/{id}/controls/{controlId}/population
//	DELETE /api/v1/audits/{id}/controls/{controlId}/population/{populationId}
//	GET    /api/v1/audits/{id}/controls/{controlId}/evidence
//	POST   /api/v1/audits/{id}/controls/{controlId}/evidence
//	DELETE /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}
//	POST   /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}/review
//	GET    /api/v1/audits/{id}/controls/{controlId}/comments
//	POST   /api/v1/audits/{id}/controls/{controlId}/comments
//	GET    /api/v1/audits/{id}/assignments
//	POST   /api/v1/audits/{id}/assignments
//	DELETE /api/v1/audits/{id}/assignments/{assignmentId}
//	GET    /api/v1/audits/{id}/trail
//	GET    /api/v1/audit/notifications
//	PATCH  /api/v1/audit/notifications/{id}/read
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	// TODO: register routes
}

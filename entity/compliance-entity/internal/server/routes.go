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

package server

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/handler"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/storage"
)

// NewRouter builds the full dependency graph (repository → service → handler),
// registers all routes, and wraps the mux with the middleware chain:
// CorrelationID → Recovery → Logger → Timeout (30 s).
//
// Authentication is handled upstream by the Choreo API Gateway; no auth
// middleware is applied here.
func NewRouter(db *sql.DB, store *storage.Service) http.Handler {
	// ── Repositories ────────────────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	auditTeamRepo := repository.NewAuditTeamRepository(db)
	auditFrameworkRepo := repository.NewAuditFrameworkRepository(db)
	frameworkControlRepo := repository.NewFrameworkControlRepository(db)
	auditProductRepo := repository.NewAuditProductRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	controlRepo := repository.NewControlRepository(db)
	evidenceRepo := repository.NewEvidenceRepository(db)
	dashboardRepo := repository.NewDashboardRepository(db)
	populationRepo := repository.NewPopulationRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	aiValidationRepo := repository.NewAIValidationRepository(db)
	auditTrailRepo := repository.NewAuditTrailRepository(db)
	auditNotificationRepo := repository.NewAuditNotificationRepository(db)
	riskTeamRepo := repository.NewRiskTeamRepository(db)
	riskScoreRepo := repository.NewRiskScoreRepository(db)
	riskCategoryRepo := repository.NewRiskCategoryRepository(db)
	riskReferenceRepo := repository.NewRiskReferenceRepository(db)
	riskRepo := repository.NewRiskRepository(db)
	riskActionPlanRepo := repository.NewRiskActionPlanRepository(db)
	riskActionStepRepo := repository.NewRiskActionStepRepository(db)
	riskComplianceRefRepo := repository.NewRiskComplianceRefRepository(db)
	riskEscalationRepo := repository.NewRiskEscalationRepository(db)
	riskChangeLogRepo := repository.NewRiskChangeLogRepository(db)
	riskEvidenceRepo := repository.NewRiskEvidenceRepository(db)
	riskAssessmentRepo := repository.NewRiskAssessmentRepository(db)
	privilegeRepo := repository.NewPrivilegeRepository(db)
	grantRepo := repository.NewGrantRepository(db)
	riskDashboardRepo := repository.NewRiskDashboardRepository(db)
	riskAnalyticsRepo := repository.NewRiskAnalyticsRepository(db)

	// ── Services ─────────────────────────────────────────────────────────────
	userSvc := service.NewCachedUserService(service.NewUserService(userRepo))
	auditTeamSvc := service.NewAuditTeamService(auditTeamRepo)
	auditFrameworkSvc := service.NewCachedAuditFrameworkService(service.NewAuditFrameworkService(auditFrameworkRepo))
	frameworkControlSvc := service.NewFrameworkControlService(frameworkControlRepo)
	auditProductSvc := service.NewAuditProductService(auditProductRepo)
	auditSvc := service.NewAuditService(auditRepo)
	controlSvc := service.NewControlService(controlRepo, frameworkControlRepo, auditRepo)
	evidenceSvc := service.NewEvidenceService(evidenceRepo)
	dashboardSvc := service.NewDashboardService(dashboardRepo)
	populationSvc := service.NewPopulationService(populationRepo)
	commentSvc := service.NewCommentService(commentRepo, auditTrailRepo)
	aiValidationSvc := service.NewAIValidationService(aiValidationRepo)
	auditTrailSvc := service.NewAuditTrailService(auditTrailRepo)
	auditNotificationSvc := service.NewAuditNotificationService(auditNotificationRepo)
	riskTeamSvc := service.NewRiskTeamService(riskTeamRepo)
	riskScoreSvc := service.NewCachedRiskScoreService(service.NewRiskScoreService(riskScoreRepo))
	riskCategorySvc := service.NewRiskCategoryService(riskCategoryRepo)
	riskReferenceSvc := service.NewRiskReferenceService(riskReferenceRepo)
	riskSvc := service.NewRiskService(riskRepo)
	riskActionStepSvc := service.NewRiskActionStepService(riskActionStepRepo, riskActionPlanRepo, riskSvc)
	riskComplianceRefSvc := service.NewRiskComplianceRefService(riskComplianceRefRepo)
	riskEscalationSvc := service.NewRiskEscalationService(riskEscalationRepo, riskSvc)
	riskChangeLogSvc := service.NewRiskChangeLogService(riskChangeLogRepo)
	// riskActionPlanSvc depends on the services above for its completion
	// cascade, which now only notifies — resolving the escalation and reverting
	// the risk moved to the escalation comment flow in the GRC backend.
	riskActionPlanSvc := service.NewRiskActionPlanService(
		riskActionPlanRepo, riskActionStepRepo, riskEvidenceRepo,
		riskEscalationSvc, riskSvc, userSvc,
	)
	riskEvidenceSvc := service.NewRiskEvidenceService(riskEvidenceRepo, riskActionPlanRepo)
	riskAssessmentSvc := service.NewRiskAssessmentService(riskAssessmentRepo)
	privilegeSvc := service.NewPrivilegeService(privilegeRepo)
	// Deliberately NOT wrapped in a cache, unlike userSvc above: grants are on
	// the revocation path and must be fresh on every request.
	grantSvc := service.NewGrantService(grantRepo)
	riskDashboardSvc := service.NewRiskDashboardService(riskDashboardRepo)
	riskAnalyticsSvc := service.NewRiskAnalyticsService(riskAnalyticsRepo)

	// ── Handlers ─────────────────────────────────────────────────────────────
	userH := handler.NewUserHandler(userSvc)
	auditTeamH := handler.NewAuditTeamHandler(auditTeamSvc)
	auditFrameworkH := handler.NewAuditFrameworkHandler(auditFrameworkSvc)
	frameworkControlH := handler.NewFrameworkControlHandler(frameworkControlSvc)
	auditProductH := handler.NewAuditProductHandler(auditProductSvc)
	auditH := handler.NewAuditHandler(auditSvc)
	controlH := handler.NewControlHandler(controlSvc)
	evidenceH := handler.NewEvidenceHandler(evidenceSvc)
	dashboardH := handler.NewDashboardHandler(dashboardSvc)
	populationH := handler.NewPopulationHandler(populationSvc)
	commentH := handler.NewCommentHandler(commentSvc)
	aiValidationH := handler.NewAIValidationHandler(aiValidationSvc)
	auditTrailH := handler.NewAuditTrailHandler(auditTrailSvc)
	auditNotificationH := handler.NewAuditNotificationHandler(auditNotificationSvc)
	riskTeamH := handler.NewRiskTeamHandler(riskTeamSvc)
	riskScoreH := handler.NewRiskScoreHandler(riskScoreSvc)
	riskCategoryH := handler.NewRiskCategoryHandler(riskCategorySvc)
	riskReferenceH := handler.NewRiskReferenceHandler(riskReferenceSvc)
	riskH := handler.NewRiskHandler(riskSvc)
	riskActionPlanH := handler.NewRiskActionPlanHandler(riskActionPlanSvc)
	riskActionStepH := handler.NewRiskActionStepHandler(riskActionStepSvc)
	riskComplianceRefH := handler.NewRiskComplianceRefHandler(riskComplianceRefSvc)
	riskEscalationH := handler.NewRiskEscalationHandler(riskEscalationSvc)
	riskChangeLogH := handler.NewRiskChangeLogHandler(riskChangeLogSvc)
	riskEvidenceH := handler.NewRiskEvidenceHandler(riskEvidenceSvc)
	riskAssessmentH := handler.NewRiskAssessmentHandler(riskAssessmentSvc)
	privilegeH := handler.NewPrivilegeHandler(privilegeSvc)
	grantH := handler.NewGrantHandler(grantSvc)
	riskDashboardH := handler.NewRiskDashboardHandler(riskDashboardSvc)
	riskAnalyticsH := handler.NewRiskAnalyticsHandler(riskAnalyticsSvc)

	// ── Routes ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handler.HealthCheck)

	// File byte-storage (Azure Blob) — the entity is the only holder of the Azure
	// account key. The GRC Backend proxies evidence/risk file bytes through these.
	// Registered only when Azure is configured (store != nil).
	if store != nil {
		fileH := handler.NewFileHandler(store)
		mux.HandleFunc("POST /files", fileH.UploadFile)
		mux.HandleFunc("GET /files", fileH.DownloadFile)
		mux.HandleFunc("GET /files/list", fileH.ListFiles)
		mux.HandleFunc("DELETE /files", fileH.DeleteFile)
		// Evidence file bytes by id (no client-supplied paths) — consumed by
		// the AI validation MCP server.
		fileContentH := handler.NewEvidenceFileContentHandler(evidenceSvc, store)
		mux.HandleFunc("GET /evidence-files/{fileId}/content", fileContentH.GetContent)
	}

	// Users
	mux.HandleFunc("POST /users/search", userH.SearchUsers)
	mux.HandleFunc("GET /users/by-email/{email}", userH.GetUserByEmail)
	mux.HandleFunc("GET /users/{id}", userH.GetUserByID)
	mux.HandleFunc("POST /users", userH.CreateUser)
	mux.HandleFunc("PATCH /users/{id}", userH.UpdateUser)

	// Role grants — who holds which role, in which scope.
	//
	// Deliberately top-level rather than nested under /users/{id}/...: Go's mux
	// cannot order "GET /users/{id}/grants" against the existing
	// "GET /users/by-email/{email}", because "/users/by-email/grants" matches
	// both and neither is more specific. That is a registration-time panic, not
	// a 404, so it takes the whole service down. The literal second segment
	// here ("by-email" vs "user") keeps every pattern unambiguous — carry it
	// forward whenever the routes below are re-added.
	//
	// The by-email read is the hot path: the GRC backend calls it on every
	// authenticated request. Its responses are never cached, so revoking a
	// grant takes effect on the caller's next request.
	mux.HandleFunc("GET /grants/by-email/{email}", grantH.GrantsByEmail)
	mux.HandleFunc("GET /grants/candidates", grantH.Candidates)
	// GrantsByUserID / CreateGrant / RevokeGrant are deliberately NOT wired up
	// yet — held back rather than shipped with no caller. This service has no
	// authorisation of its own (see internal/middleware — correlation ID,
	// logging, recovery, timeout, and a header pass-through for attribution;
	// nothing checks who's calling), so it trusts network/gateway placement
	// entirely. Exposing role-mutation endpoints with zero consumer would
	// widen that trust boundary for no product benefit. Re-add
	// "GET /grants/user/{id}", "POST /grants/user/{id}", and
	// "DELETE /grants/user/{id}/{grantId}?revokedBy=" (grantH.GrantsByUserID /
	// grantH.CreateGrant / grantH.RevokeGrant) in the same PR that lands the
	// MANAGE_USERS-gated admin grant editor that will actually call them.
	mux.HandleFunc("GET /roles", grantH.ListRoles)

	// Audit teams
	mux.HandleFunc("POST /audit/teams/search", auditTeamH.SearchAuditTeams)
	mux.HandleFunc("GET /audit/teams/{id}", auditTeamH.GetAuditTeamByID)
	mux.HandleFunc("POST /audit/teams", auditTeamH.CreateAuditTeam)
	mux.HandleFunc("PATCH /audit/teams/{id}", auditTeamH.UpdateAuditTeam)

	// Audit frameworks
	mux.HandleFunc("POST /audit/frameworks/search", auditFrameworkH.SearchAuditFrameworks)
	mux.HandleFunc("GET /audit/frameworks/{id}", auditFrameworkH.GetAuditFrameworkByID)
	mux.HandleFunc("POST /audit/frameworks", auditFrameworkH.CreateAuditFramework)
	mux.HandleFunc("PATCH /audit/frameworks/{id}", auditFrameworkH.UpdateAuditFramework)

	// Framework control library (versioned, immutable; nested under frameworks)
	mux.HandleFunc("GET /audit/frameworks/{id}/controls", frameworkControlH.ListCurrentControls)
	mux.HandleFunc("POST /audit/frameworks/{id}/controls", frameworkControlH.CreateControl)
	mux.HandleFunc("PUT /audit/frameworks/{id}/controls/{controlId}", frameworkControlH.NewVersion)
	mux.HandleFunc("GET /audit/frameworks/{id}/controls/{controlNumber}/versions", frameworkControlH.ListAllVersions)

	// Audit products
	mux.HandleFunc("POST /audit/products/search", auditProductH.SearchAuditProducts)
	mux.HandleFunc("GET /audit/products/{id}", auditProductH.GetAuditProductByID)
	mux.HandleFunc("POST /audit/products", auditProductH.CreateAuditProduct)
	mux.HandleFunc("PATCH /audit/products/{id}", auditProductH.UpdateAuditProduct)

	// Audits
	mux.HandleFunc("POST /audits/search", auditH.SearchAudits)
	mux.HandleFunc("GET /audits/{id}", auditH.GetAuditByID)
	mux.HandleFunc("POST /audits", auditH.CreateAudit)
	mux.HandleFunc("PATCH /audits/{id}", auditH.UpdateAudit)
	mux.HandleFunc("DELETE /audits/{id}", auditH.DeleteAudit)

	// Audit trail (append-only; write from external callers, read for timeline UI)
	mux.HandleFunc("POST /audits/{auditId}/trail", auditTrailH.CreateAuditTrail)
	mux.HandleFunc("GET /audits/{auditId}/trail", auditTrailH.ListAuditTrail)

	// Audit notifications (send-log for every audit-module email; also the
	// reminder job's de-dup mechanism)
	mux.HandleFunc("POST /audit/notifications", auditNotificationH.CreateAuditNotification)
	mux.HandleFunc("POST /audit/notifications/exists", auditNotificationH.CheckAuditNotificationExists)

	// Controls (cross-audit search; nested CRUD under audits)
	mux.HandleFunc("POST /audit/dashboard/search", dashboardH.GetDashboard)
	mux.HandleFunc("POST /audit/work-queue/search", dashboardH.GetWorkQueuePage)
	// Resource-level assignment check (returns the derived audit id, used by the
	// web app's own evidence-submission IDOR gate) and active-population lookup
	// for OE controls.
	mux.HandleFunc("GET /audit-controls/{controlId}/evidence-assignment", controlH.GetEvidenceAssignment)
	mux.HandleFunc("GET /audit-controls/{controlId}/active-population", controlH.GetActivePopulation)
	mux.HandleFunc("POST /controls/search", controlH.SearchControlsGlobal)
	mux.HandleFunc("POST /audits/{auditId}/controls/search", controlH.SearchControls)
	mux.HandleFunc("POST /audits/{auditId}/controls/bulk", controlH.BulkCreateControls)
	mux.HandleFunc("GET /audits/{auditId}/controls/{controlId}", controlH.GetControlByID)
	mux.HandleFunc("POST /audits/{auditId}/controls", controlH.CreateControl)
	mux.HandleFunc("PATCH /audits/{auditId}/controls/{controlId}", controlH.UpdateControl)
	mux.HandleFunc("POST /audits/{auditId}/controls/{controlId}/status-override", controlH.OverrideControlStatus)
	mux.HandleFunc("DELETE /audits/{auditId}/controls/{controlId}", controlH.DeleteControl)

	// Evidence (nested creation/list under controls; flat access by evidence ID)
	mux.HandleFunc("POST /audits/{auditId}/controls/{controlId}/evidence", evidenceH.CreateEvidence)
	mux.HandleFunc("GET /audits/{auditId}/controls/{controlId}/evidence", evidenceH.ListEvidenceByControl)
	mux.HandleFunc("GET /evidence/{evidenceId}", evidenceH.GetEvidenceByID)
	mux.HandleFunc("PATCH /evidence/{evidenceId}", evidenceH.UpdateEvidence)
	mux.HandleFunc("POST /evidence/{evidenceId}/files", evidenceH.AddEvidenceFile)
	mux.HandleFunc("GET /evidence/{evidenceId}/files", evidenceH.ListEvidenceFiles)
	// Distinct prefix (not /evidence/...) to avoid a routing conflict with the
	// list-files pattern above.
	mux.HandleFunc("GET /evidence-files/{fileId}", evidenceH.GetEvidenceFileByID)
	mux.HandleFunc("DELETE /evidence-files/{fileId}", evidenceH.DeleteEvidenceFile)
	mux.HandleFunc("DELETE /evidence/{evidenceId}", evidenceH.DeleteEvidence)

	// Control comments (control-scoped — one thread per control, spanning
	// population + evidence phases; flat delete by comment ID)
	mux.HandleFunc("POST /audits/{auditId}/controls/{controlId}/comments", commentH.CreateComment)
	mux.HandleFunc("GET /audits/{auditId}/controls/{controlId}/comments", commentH.ListComments)
	mux.HandleFunc("DELETE /comments/{commentId}", commentH.DeleteComment)

	// Evidence AI validation log (written by the async validation agent; read as review hints)
	mux.HandleFunc("POST /evidence/{evidenceId}/ai-validations", aiValidationH.CreateValidation)
	mux.HandleFunc("GET /evidence/{evidenceId}/ai-validations", aiValidationH.ListValidations)

	// Populations (nested creation under controls; flat access by population ID)
	mux.HandleFunc("POST /audits/{auditId}/controls/{controlId}/populations", populationH.CreatePopulation)
	mux.HandleFunc("GET /audits/{auditId}/controls/{controlId}/populations", populationH.ListPopulations)
	mux.HandleFunc("GET /populations/{populationId}", populationH.GetPopulationByID)
	mux.HandleFunc("PATCH /populations/{populationId}", populationH.UpdatePopulation)
	mux.HandleFunc("POST /populations/{populationId}/files", populationH.AddPopulationFile)
	mux.HandleFunc("GET /populations/{populationId}/files", populationH.ListPopulationFiles)
	// Top-level (not nested under /populations/) so it can never collide with
	// /populations/{populationId}/files — same shape as /evidence-files/{fileId}.
	mux.HandleFunc("GET /population-files/{fileId}", populationH.GetPopulationFileByID)
	mux.HandleFunc("DELETE /population-files/{fileId}", populationH.DeletePopulationFile)

	// Risk teams
	mux.HandleFunc("POST /risk/teams/search", riskTeamH.SearchRiskTeams)
	mux.HandleFunc("GET /risk/teams/{id}", riskTeamH.GetRiskTeamByID)
	mux.HandleFunc("POST /risk/teams", riskTeamH.CreateRiskTeam)
	mux.HandleFunc("PATCH /risk/teams/{id}", riskTeamH.UpdateRiskTeam)

	// Risk scores (reference data — read only)
	mux.HandleFunc("GET /risk/scores", riskScoreH.ListRiskScores)
	mux.HandleFunc("GET /risk/categories", riskCategoryH.ListRiskCategories)

	// Risk compliance references
	mux.HandleFunc("POST /risk/compliance-references/search", riskReferenceH.SearchRiskReferences)
	mux.HandleFunc("GET /risk/compliance-references/{id}", riskReferenceH.GetRiskReferenceByID)
	mux.HandleFunc("POST /risk/compliance-references", riskReferenceH.CreateRiskReference)
	mux.HandleFunc("PATCH /risk/compliance-references/{id}", riskReferenceH.UpdateRiskReference)

	// Risks
	mux.HandleFunc("GET /risks/next-sequence-number", riskH.NextSequenceNumber)
	mux.HandleFunc("GET /role-privileges", privilegeH.RolePrivilegeMap)
	mux.HandleFunc("POST /risk/dashboard/search", riskDashboardH.Summary)
	mux.HandleFunc("POST /risk/analytics/search", riskAnalyticsH.Summary)
	mux.HandleFunc("POST /risks/search", riskH.SearchRisks)
	mux.HandleFunc("GET /risks/{id}", riskH.GetRiskByID)
	mux.HandleFunc("GET /risks/{id}/detail", riskH.GetRiskDetail)
	mux.HandleFunc("POST /risks", riskH.CreateRisk)
	mux.HandleFunc("PATCH /risks/{id}", riskH.UpdateRisk)

	// Risk action plans (nested creation/list under risk; flat get/patch at top-level)
	mux.HandleFunc("POST /risks/{riskId}/action-plans", riskActionPlanH.CreateRiskActionPlan)
	mux.HandleFunc("GET /risks/{riskId}/action-plans", riskActionPlanH.ListRiskActionPlans)
	mux.HandleFunc("GET /action-plans/{planId}", riskActionPlanH.GetRiskActionPlanByID)
	mux.HandleFunc("PATCH /action-plans/{planId}", riskActionPlanH.UpdateRiskActionPlan)
	mux.HandleFunc("POST /action-plans/{planId}/complete", riskActionPlanH.CompleteRiskActionPlan)

	// Risk action steps (nested under action plans)
	mux.HandleFunc("POST /action-plans/{planId}/steps", riskActionStepH.CreateRiskActionStep)
	mux.HandleFunc("GET /action-plans/{planId}/steps", riskActionStepH.ListRiskActionSteps)
	mux.HandleFunc("GET /action-plans/{planId}/steps/{stepId}", riskActionStepH.GetRiskActionStepByID)
	mux.HandleFunc("PATCH /action-plans/{planId}/steps/{stepId}", riskActionStepH.UpdateRiskActionStep)
	mux.HandleFunc("DELETE /action-plans/{planId}/steps/{stepId}", riskActionStepH.DeleteRiskActionStep)

	// Risk compliance reference links (junction: risk ↔ risk_security_compliance_reference)
	mux.HandleFunc("POST /risks/{riskId}/compliance-references", riskComplianceRefH.AddRiskComplianceRef)
	mux.HandleFunc("GET /risks/{riskId}/compliance-references", riskComplianceRefH.ListRiskComplianceRefs)
	mux.HandleFunc("DELETE /risks/{riskId}/compliance-references/{referenceId}", riskComplianceRefH.DeleteRiskComplianceRef)

	// Risk escalations (nested under risks)
	mux.HandleFunc("POST /risks/{riskId}/escalate", riskEscalationH.EscalateRisk)
	mux.HandleFunc("POST /risks/{riskId}/escalations", riskEscalationH.CreateRiskEscalation)
	mux.HandleFunc("GET /risks/{riskId}/escalations", riskEscalationH.ListRiskEscalations)
	mux.HandleFunc("GET /risks/{riskId}/escalations/{escalationId}", riskEscalationH.GetRiskEscalationByID)
	mux.HandleFunc("PATCH /risks/{riskId}/escalations/{escalationId}", riskEscalationH.UpdateRiskEscalation)
	mux.HandleFunc("PATCH /risks/{riskId}/escalations/{escalationId}/comment", riskEscalationH.CommentEscalation)

	// Risk change log (audit trail for risks; append-only)
	mux.HandleFunc("POST /risks/{riskId}/changes", riskChangeLogH.CreateRiskChangeLog)
	mux.HandleFunc("GET /risks/{riskId}/changes", riskChangeLogH.ListRiskChangeLog)

	// Risk evidence files (nested under risks)
	mux.HandleFunc("POST /risks/{riskId}/evidence", riskEvidenceH.CreateRiskEvidence)
	mux.HandleFunc("GET /risks/{riskId}/evidence", riskEvidenceH.ListRiskEvidence)
	mux.HandleFunc("GET /risk-evidence/{fileId}", riskEvidenceH.GetRiskEvidence)
	mux.HandleFunc("DELETE /risks/{riskId}/evidence/{fileId}", riskEvidenceH.DeleteRiskEvidence)

	// Risk assessments (nested under risks)
	mux.HandleFunc("POST /risks/{riskId}/assessments", riskAssessmentH.CreateRiskAssessment)
	mux.HandleFunc("GET /risks/{riskId}/assessments", riskAssessmentH.ListRiskAssessments)

	// ── Middleware chain ──────────────────────────────────────────────────────
	// Order (outermost first): CorrelationID → Recovery → Logger → UserIDToken
	// → Timeout. UserIDToken captures the backend-forwarded x-user-id-token so
	// services can attribute writes to the verified actor.
	return middleware.CorrelationID(
		middleware.Recovery(
			middleware.Logger(
				middleware.UserIDToken(
					middleware.Timeout(30 * time.Second)(mux),
				),
			),
		),
	)
}

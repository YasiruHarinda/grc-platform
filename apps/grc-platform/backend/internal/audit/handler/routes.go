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
	"context"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aiagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
)

// Deps holds all service dependencies for Audit Hub handlers.
// Handlers call service methods; services call repositories.
type Deps struct {
	Audit        auditservice.AuditService
	Control      auditservice.ControlService
	Framework    auditservice.FrameworkService
	User         auditservice.UserService
	Team         auditservice.TeamService
	Dashboard    auditservice.DashboardService
	Evidence     auditservice.EvidenceService
	Population   auditservice.PopulationService
	Comment      auditservice.CommentService
	Notification auditservice.NotificationService
	Assignment   auditservice.AssignmentService
	Trail        auditservice.TrailService
	AIValidation auditservice.AIValidationService

	// AIAgent triggers async AI validation after an evidence submission.
	// Nil when AI_VALIDATION_ENABLED is false — the trigger becomes a no-op.
	AIAgent *aiagent.Client

	// Email sends every audit-module notification (owner assigned, reminder
	// digest, resubmission needed, sample submitted) — see notify.go.
	Email *emailer.Client
	// Users resolves an owner_id/actor email to a deliverable address and
	// display name for notify.go. Distinct from the User service (which
	// backs the owner/auditor dropdown UI) — this is the same repository,
	// used for direct lookups instead of listing.
	Users repository.UserRepository
	// FrontendBaseURL builds the "View in Audit Hub" link inside notification
	// emails.
	FrontendBaseURL string
	// TriggerReminderJob runs the daily due-date reminder sweep on demand —
	// wired in cmd/server/main.go to the reminder job's RunOnce method, kept
	// as a plain function here so this package never imports internal/audit/job
	// (which would import back into handler and cycle). Nil disables the
	// manual-trigger endpoint.
	TriggerReminderJob func(ctx context.Context) error
}

// RegisterRoutes mounts all Audit Hub routes onto mux.
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	ah := &auditHandler{svc: deps.Audit}
	ch := &controlHandler{svc: deps.Control, notify: &deps}
	tlh := &trailHandler{svc: deps.Trail}
	fh := &frameworkHandler{svc: deps.Framework}
	uh := &userHandler{svc: deps.User}
	th := &teamHandler{svc: deps.Team}
	dh := &dashboardHandler{svc: deps.Dashboard}
	eh := &evidenceHandler{svc: deps.Evidence, controlSvc: deps.Control, popSvc: deps.Population, trailSvc: deps.Trail, aiClient: deps.AIAgent, notify: &deps}
	cmh := &commentHandler{svc: deps.Comment}
	avh := &aiValidationHandler{svc: deps.AIValidation}
	rjh := &reminderJobHandler{trigger: deps.TriggerReminderJob}

	// Current user (shared by both hubs — resolved privilege set unions RISK_*
	// and AUDIT_* names).
	mux.HandleFunc("GET /api/v1/me/privileges", deps.handleGetMyPrivileges)

	// Dashboard.
	mux.HandleFunc("GET /api/v1/audit/dashboard", dh.getDashboard)
	mux.HandleFunc("GET /api/v1/audit/work-queue", dh.getWorkQueue)

	// Lookup data for Create Audit form dropdowns.
	mux.HandleFunc("GET /api/v1/audit/frameworks", fh.listFrameworks)
	mux.HandleFunc("POST /api/v1/audit/frameworks", fh.createFramework)
	mux.HandleFunc("GET /api/v1/audit/frameworks/{id}/controls", fh.listFrameworkControls)
	mux.HandleFunc("POST /api/v1/audit/frameworks/{id}/controls", fh.createFrameworkControl)
	mux.HandleFunc("GET /api/v1/audit/products", fh.listProducts)
	mux.HandleFunc("POST /api/v1/audit/products", fh.createProduct)
	mux.HandleFunc("GET /api/v1/audit/users", uh.listUsers)
	mux.HandleFunc("GET /api/v1/audit/teams", th.listTeams)

	// Manual trigger for the daily due-date reminder digest — QA/ops
	// convenience so the job can be tested/re-run without waiting for its
	// fixed daily time or restarting the server.
	mux.HandleFunc("POST /api/v1/audits/reminders/run", rjh.run)

	// Audit CRUD.
	mux.HandleFunc("GET /api/v1/audits", ah.listAudits)
	mux.HandleFunc("POST /api/v1/audits", ah.createAudit)
	mux.HandleFunc("GET /api/v1/audits/{id}", ah.getAudit)
	mux.HandleFunc("PUT /api/v1/audits/{id}", ah.updateAudit)
	mux.HandleFunc("DELETE /api/v1/audits/{id}", ah.deleteAudit)

	// Control CRUD + status transitions.
	// Note: /bulk must be registered before /{controlId} so the router matches it first.
	mux.HandleFunc("GET /api/v1/audits/{id}/controls", ch.listControls)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls", ch.addControl)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/bulk", ch.bulkAddControls)
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}", ch.getControl)
	mux.HandleFunc("PUT /api/v1/audits/{id}/controls/{controlId}", ch.updateControl)
	mux.HandleFunc("DELETE /api/v1/audits/{id}/controls/{controlId}", ch.deleteControl)
	mux.HandleFunc("PATCH /api/v1/audits/{id}/controls/{controlId}/status", ch.updateControlStatus)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/status/override", ch.overrideControlStatus)
	// Immutable per-control history (append-only audit_trail) for the History tab.
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/trail", tlh.listControlTrail)
	// Audit-wide activity log (audit-level events + every control's events, filterable).
	mux.HandleFunc("GET /api/v1/audits/{id}/trail", tlh.listAuditTrail)

	// Evidence submission (backend-proxied upload flow).
	// Note: /upload-link, /upload and /submit must be registered before the plain
	// /evidence list route so the router matches their literal suffixes first.
	// File bytes are proxied through the backend (POST /upload, multipart) — no
	// write SAS is handed to the client.
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/evidence/upload-link", eh.getUploadLink)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/upload", eh.uploadEvidence)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/submit", eh.submitEvidence)
	// Appends to the current (still-SUBMITTED) round instead of starting a new
	// one — what "Add Files" uses while a submission awaits internal review.
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/files", eh.addEvidenceFiles)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/withdraw", eh.withdrawEvidence)
	// Internal-reviewer and auditor decisions on the evidence round (mirrors the
	// population/review + population/validate pair below) — both record the
	// reviewed round's own status, not just the control's.
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/review", eh.reviewEvidence)
	// Auditor-gated decision at EVIDENCE_UNDER_VALIDATION (assigned auditor POC only —
	// see requireAssignedAuditor).
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/evidence/validate", eh.validateEvidence)
	// Audit-scoped file delete: knows the control, so it can send an emptied
	// submission back to EVIDENCE_PENDING (see deleteControlEvidenceFile).
	mux.HandleFunc("DELETE /api/v1/audits/{id}/controls/{controlId}/evidence/files/{fileId}", eh.deleteControlEvidenceFile)
	// Whole-round delete for a fileless (attestation-only) completion — no
	// individual file exists to delete instead (see deleteEvidenceRound).
	mux.HandleFunc("DELETE /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}", eh.deleteEvidenceRound)
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/evidence", eh.listEvidence)
	// Proxied file download by file ID (bytes streamed via the Compliance Entity).
	mux.HandleFunc("GET /api/v1/evidence/files/{fileId}/download", eh.downloadEvidenceFile)
	// Remove a single file from an evidence submission (DB record only).
	mux.HandleFunc("DELETE /api/v1/evidence/files/{fileId}", eh.deleteEvidenceFile)

	// Population submission (OE controls; same proxied upload flow as evidence).
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/population/upload-link", eh.getPopulationUploadLink)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/population/upload", eh.uploadPopulation)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/population/submit", eh.submitPopulation)
	// Internal-reviewer and auditor decisions on the population round.
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/population/review", eh.reviewPopulation)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/population/validate", eh.validatePopulation)
	// View the current round + its files (split population[] / sample[]) and the
	// auditor's sample note — team, reviewer, auditor, or admin (see canViewPopulation).
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/population", eh.listPopulation)
	// Remove a population/sample file (see deletePopulationFile for the per-kind gate).
	mux.HandleFunc("DELETE /api/v1/audits/{id}/controls/{controlId}/population/files/{fileId}", eh.deletePopulationFile)
	// Blank the team's population-submission note without touching files/status
	// (see deletePopulationAttestation) — the round itself is never deleted.
	mux.HandleFunc("DELETE /api/v1/audits/{id}/controls/{controlId}/population/attestation", eh.deletePopulationAttestation)
	// Proxied file download by file ID (mirrors the evidence download route).
	mux.HandleFunc("GET /api/v1/population/files/{fileId}/download", eh.downloadPopulationFile)

	// Sample selection (auditor-gated — see requireAssignedAuditor). Reuses the
	// population round; files land in a "sample/" subfolder of it.
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/sample/upload-link", eh.getSampleUploadLink)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/sample/upload", eh.uploadSample)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/sample/submit", eh.submitSample)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/sample/request-time", eh.requestSampleTime)

	// Control comments (one thread per control, spanning population + evidence
	// phases — available as soon as the control drawer is open, not gated on an
	// evidence/population round existing yet; is_internal hides from external auditors)
	mux.HandleFunc("GET /api/v1/audits/{id}/controls/{controlId}/comments", cmh.listComments)
	mux.HandleFunc("POST /api/v1/audits/{id}/controls/{controlId}/comments", cmh.addComment)
	mux.HandleFunc("DELETE /api/v1/audits/{id}/controls/{controlId}/comments/{commentId}", cmh.deleteComment)

	// AI validation advisory results (read-only hint; SUBMIT or REVIEW evidence).
	mux.HandleFunc("GET /api/v1/evidence/{evidenceId}/ai-validations", avh.listValidations)
}

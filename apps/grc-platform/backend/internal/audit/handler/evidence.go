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

package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aiagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// maxEvidenceUploadBytes caps a single proxied evidence upload (bytes travel
// through the backend, so bound them to protect memory and the gateway).
const maxEvidenceUploadBytes = 25 << 20 // 25 MiB

// channelWebApp tags audit-trail entries as originating from the GRC web app,
// distinguishing them from other submission channels.
const channelWebApp = "web-app"

// fileNamesOf extracts file names for RecordEvidenceAction's fileNames param.
func fileNamesOf(files []*model.AuditEvidenceFile) []string {
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.FileName)
	}
	return names
}

// trailFileNames is fileNamesOf, except a fileless round (attestation-only)
// logs the attestation text instead of an empty file list — otherwise the
// trail would record a submission with nothing attached to it.
func trailFileNames(evidence *model.AuditEvidence) []string {
	if len(evidence.Files) == 0 && evidence.Attestation != "" {
		return []string{evidence.Attestation}
	}
	return fileNamesOf(evidence.Files)
}

// recordEvidenceTrail appends a best-effort attribution entry. Failures are logged
// and swallowed — they never affect the submission the user just made. fileNames
// is nil for calls that have nothing file-shaped to attach (population/sample).
func recordEvidenceTrail(ctx context.Context, trailSvc service.TrailService, auditID, controlID, evidenceID int, actor, via, issuer string, fileNames []string) {
	if trailSvc == nil {
		return
	}
	if err := trailSvc.RecordEvidenceAction(ctx, auditID, controlID, evidenceID, "UPLOADED", actor, via, issuer, fileNames); err != nil {
		slog.WarnContext(ctx, "audit-trail attribution failed", "controlId", controlID, "via", via, "err", err)
	}
}

// readUpload parses a bounded multipart upload (folderPath + file), returning the
// folder path, base file name, sniffed content type, and bytes. It writes the error
// response and returns ok=false on any failure. Shared by the population and
// sample upload routes.
func readUpload(w http.ResponseWriter, r *http.Request) (folderPath, fileName, contentType string, data []byte, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEvidenceUploadBytes)
	if err := r.ParseMultipartForm(maxEvidenceUploadBytes); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		response.WriteError(w, http.StatusRequestEntityTooLarge, "file too large or malformed upload (max 25 MB)")
		return "", "", "", nil, false
	}
	folderPath = r.FormValue("folderPath")
	f, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "file is required")
		return "", "", "", nil, false
	}
	defer f.Close()

	data, err = io.ReadAll(f)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "could not read uploaded file")
		return "", "", "", nil, false
	}
	contentType = header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	fileName = filepath.Base(header.Filename)
	if err := validateUploadFileType(fileName, contentType); err != nil {
		response.WriteError(w, http.StatusBadRequest, err.Error())
		return "", "", "", nil, false
	}
	return folderPath, fileName, contentType, data, true
}

type evidenceHandler struct {
	svc        service.EvidenceService
	controlSvc service.ControlService
	// popSvc records population submissions (web-app population routes).
	popSvc service.PopulationService
	// trailSvc records best-effort attribution entries on submit. May be nil.
	trailSvc service.TrailService
	// aiClient triggers async AI validation after a submission. It is nil when
	// AI_VALIDATION_ENABLED is false, which disables the trigger entirely.
	aiClient *aiagent.Client
	// notify sends resubmission-needed and sample-submitted notification
	// emails from decideRound and submitSample — see notify.go.
	notify    *Deps
	directory *directory.Service
}

// resolveEvidenceSubmitters batch-resolves each round's CreatedByName from
// CreatedBy (the submitter's raw uuid) — see AuditTrailEntry.CreatedByName
// for the same pattern.
func (h *evidenceHandler) resolveEvidenceSubmitters(ctx context.Context, evidence []*model.AuditEvidence) {
	uuids := make([]string, 0, len(evidence))
	for _, e := range evidence {
		if e.CreatedBy != "" {
			uuids = append(uuids, e.CreatedBy)
		}
	}
	people := h.directory.LookupAll(ctx, uuids)
	for _, e := range evidence {
		p, ok := people[e.CreatedBy]
		if !ok {
			e.CreatedByName = e.CreatedBy
			continue
		}
		switch {
		case strings.TrimSpace(p.DisplayName) != "":
			e.CreatedByName = strings.TrimSpace(p.DisplayName)
		case p.Email != "":
			e.CreatedByName = p.Email
		default:
			e.CreatedByName = e.CreatedBy
		}
	}
}

// requireAssignment enforces resource-level authorization for the web-app evidence
// routes: the caller must be assigned to controlID for an actionable
// status (else 403), and the route's audit id must equal the server-derived audit
// id (else 404 — a client cannot aim at another audit's control). It returns the
// derived audit id and ok=false after writing the response on failure.
//
// Users who hold ManageControls (compliance admin) or ViewAllAudits (org-wide
// read, e.g. compliance team) bypass the owner-assignment check —
// they already have full or org-wide read/write over audit data, so the IDOR
// restriction is redundant and would block legitimate submissions. Both
// privileges can be granted scoped to a single team, though (module=AUDIT), so
// the bypass is checked with HasPrivilegeIn against controlID's own team —
// never the unscoped HasPrivilege, which would let a team-scoped grant bypass
// the check for every other team's controls too.
func (h *evidenceHandler) requireAssignment(w http.ResponseWriter, r *http.Request, auditID, controlID int) bool {
	if auth.HasPrivilege(r.Context(), privilege.ManageControls) || auth.HasPrivilege(r.Context(), privilege.ViewAllAudits) {
		control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return false
		}
		teamID := 0
		if control.TeamID != nil {
			teamID = *control.TeamID
		}
		if auth.HasPrivilegeIn(r.Context(), privilege.ManageControls, teamID) ||
			auth.HasPrivilegeIn(r.Context(), privilege.ViewAllAudits, teamID) {
			return true
		}
	}
	actor := auth.FromContext(r.Context())
	derived, found, err := h.controlSvc.AssignedAuditID(r.Context(), actor.UserID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return false
	}
	if !found {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return false
	}
	if derived != auditID {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return false
	}
	return true
}

// getUploadLink handles GET /api/v1/audits/{id}/controls/{controlId}/evidence/upload-link.
//
// Returns this control's evidence folder path — a human-readable, deterministic
// prefix the client uses on every subsequent upload/submit call for this round.
func (h *evidenceHandler) getUploadLink(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}

	link, err := h.svc.GetUploadLink(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, link)
}

// uploadEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/upload.
//
// The client sends the file as multipart/form-data (fields: folderPath, file).
// The backend validates size/type and proxies the bytes to Azure using its own
// account key — no SAS is ever handed to the client, so the byte transfer stays
// client -> backend (Untrust -> Trust) then backend -> Azure (Trust -> Untrust).
func (h *evidenceHandler) uploadEvidence(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}

	// Bound the request body before parsing to protect memory and the gateway.
	r.Body = http.MaxBytesReader(w, r.Body, maxEvidenceUploadBytes)
	if err := r.ParseMultipartForm(maxEvidenceUploadBytes); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		response.WriteError(w, http.StatusRequestEntityTooLarge, "file too large or malformed upload (max 25 MB)")
		return
	}

	folderPath := r.FormValue("folderPath")
	// Bind the path exactly to this control's evidence folder (auditID/controlID
	// are server-derived; the folder name comes from the audit/control lookup).
	if err := h.svc.ValidateEvidenceFolderPath(r.Context(), auditID, controlID, folderPath); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	f, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}

	// Resolve content type from the part header, sniffing the bytes as a fallback
	// rather than blindly trusting the client-declared type.
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	// Strip any client-supplied path; keep only the base file name.
	fileName := filepath.Base(header.Filename)

	if err := validateUploadFileType(fileName, contentType); err != nil {
		response.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	blobName, err := h.svc.UploadFile(r.Context(), folderPath, fileName, contentType, data)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	response.WriteJSONValue(w, http.StatusCreated, map[string]any{
		"fileName": fileName,
		"blobName": blobName,
		"size":     len(data),
	})
}

// submitEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/submit.
//
// The client has already uploaded each file via the upload endpoint and
// accumulated their returned blob names. This endpoint records exactly that
// list in the DB and advances the control status to EVIDENCE_INTERNAL_REVIEW.
func (h *evidenceHandler) submitEvidence(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}
	if h.requireControlNotComplete(w, r, auditID, controlID) == nil {
		return
	}

	var req model.SubmitEvidenceRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	user := auth.FromContext(r.Context())
	actor := user.Subject
	isAdmin := auth.HasPrivilege(r.Context(), privilege.ManageControls)

	evidence, err := h.svc.Submit(r.Context(), auditID, controlID, req.Files, req.Attestation, isAdmin, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	// Advance the control to EVIDENCE_INTERNAL_REVIEW now that files are recorded.
	statusReq := model.UpdateStatusRequest{Status: "EVIDENCE_INTERNAL_REVIEW"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID); err == nil && control != nil {
		h.notify.notifyControlStatusReached(r.Context(), control, "EVIDENCE_INTERNAL_REVIEW", actor)
	}

	// Best-effort audit-trail attribution: this submission came through the web
	// app. A fileless round has no file names to log, so log the attestation text
	// instead.
	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, evidence.ID, actor, channelWebApp, user.Issuer, trailFileNames(evidence))

	// Fire-and-forget AI validation — skipped for a fileless round, which has
	// nothing for the validator to analyze. Detached from the request context (a
	// client disconnect must not cancel it) and best-effort — a failure here
	// never affects the submission the user just made.
	if len(evidence.Files) > 0 {
		h.triggerAIValidation(auditID, controlID, evidence.ID, actor)
	}

	response.WriteJSONValue(w, http.StatusCreated, evidence)
}

// addEvidenceFiles handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/files.
//
// Appends files to the control's CURRENT evidence round (must still be
// SUBMITTED, i.e. under internal review) instead of starting a new one via
// Submit. This is what "Add Files" uses: before this endpoint existed, Add
// Files called the same submit path as the initial submission, which created
// a brand-new round every time — and since a reviewer's later decision only
// closes out the single latest round, any earlier round created by an
// in-review "Add Files" click was left stranded in SUBMITTED forever,
// resurfacing its files alongside every future resubmission.
func (h *evidenceHandler) addEvidenceFiles(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}
	if h.requireControlNotComplete(w, r, auditID, controlID) == nil {
		return
	}

	var req model.SubmitEvidenceRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	user := auth.FromContext(r.Context())
	actor := user.Subject

	evidence, err := h.svc.AddFiles(r.Context(), auditID, controlID, req.Files, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	// Best-effort audit-trail attribution: this submission came through the web app.
	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, evidence.ID, actor, channelWebApp, user.Issuer, fileNamesOf(evidence.Files))

	// Re-run AI validation now that more files are attached — same best-effort,
	// fire-and-forget semantics as the initial submission.
	h.triggerAIValidation(auditID, controlID, evidence.ID, actor)

	response.WriteJSONValue(w, http.StatusOK, evidence)
}

// withdrawEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/withdraw.
//
// Lets the submitter pull a submission back while it is still in internal
// review (EVIDENCE_INTERNAL_REVIEW → EVIDENCE_PENDING) so files can be edited
// and resubmitted. Only the creator of the latest submission round (or a user
// holding ManageControls) may withdraw; once review has moved past internal
// review the submission is locked.
func (h *evidenceHandler) withdrawEvidence(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}

	control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control == nil {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	if control.Status != "EVIDENCE_INTERNAL_REVIEW" {
		response.WriteError(w, http.StatusConflict, "submission can only be withdrawn while it is under internal review")
		return
	}

	actor := auth.FromContext(r.Context()).Subject

	// Resource-level check: the caller must own the latest submission round.
	// ManageControls holders (compliance admin) can withdraw any submission.
	if !auth.HasPrivilege(r.Context(), privilege.ManageControls) {
		evidence, err := h.svc.List(r.Context(), auditID, controlID)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if len(evidence) == 0 || evidence[0].CreatedBy != actor {
			response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
			return
		}
	}

	statusReq := model.UpdateStatusRequest{Status: "EVIDENCE_PENDING"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"status": "EVIDENCE_PENDING"})
}

// reviewEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/review.
//
// The internal reviewer's decision on a submission under EVIDENCE_INTERNAL_REVIEW:
// approve advances it to auditor validation; reject sends the team back to
// EVIDENCE_PENDING to resubmit. Either way, the reviewed round's own status is
// recorded (COMPLIANCE_APPROVED/COMPLIANCE_REJECTED) — mirrors reviewPopulation
// — so a later resubmission's round is never conflated with this one in the
// live evidence view (see SubmittedEvidenceList, which hides rejected rounds).
func (h *evidenceHandler) reviewEvidence(w http.ResponseWriter, r *http.Request) {
	h.decideRound(w, r, decideRoundParams{
		preGate: func(w http.ResponseWriter, r *http.Request) bool {
			return auth.RequireAnyPrivilege(r.Context(), w, privilege.ReviewEvidence, privilege.ManageControls)
		},
		requiredStatus:    "EVIDENCE_INTERNAL_REVIEW",
		statusConflictMsg: "evidence can only be reviewed while it is under internal review",
		latestRoundID: func(ctx context.Context, auditID, controlID int) (int, error) {
			round, err := h.svc.LatestRound(ctx, auditID, controlID)
			if err != nil {
				return 0, err
			}
			return round.ID, nil
		},
		updateRoundStatus:    h.svc.UpdateRoundStatus,
		approveRoundStatus:   "COMPLIANCE_APPROVED",
		approveControlStatus: "EVIDENCE_UNDER_VALIDATION",
		rejectRoundStatus:    "COMPLIANCE_REJECTED",
		rejectControlStatus:  "EVIDENCE_PENDING",
	})
}

// validateEvidence handles POST /api/v1/audits/{id}/controls/{controlId}/evidence/validate.
//
// The assigned auditor's decision on evidence that passed internal review:
// approve closes the control out; reject sends it back to the team. Either way
// the reviewed round's own status is recorded (APPROVED/AUDITOR_REJECTED), same
// reasoning as reviewEvidence above. Auditor-gated (see requireAssignedAuditor).
func (h *evidenceHandler) validateEvidence(w http.ResponseWriter, r *http.Request) {
	h.decideRound(w, r, decideRoundParams{
		postGate:          assignedAuditorGate(privilege.ValidateEvidence),
		requiredStatus:    "EVIDENCE_UNDER_VALIDATION",
		statusConflictMsg: "evidence can only be validated while it is under auditor validation",
		latestRoundID: func(ctx context.Context, auditID, controlID int) (int, error) {
			round, err := h.svc.LatestRound(ctx, auditID, controlID)
			if err != nil {
				return 0, err
			}
			return round.ID, nil
		},
		updateRoundStatus:    h.svc.UpdateRoundStatus,
		approveRoundStatus:   "APPROVED",
		approveControlStatus: "COMPLETE",
		rejectRoundStatus:    "AUDITOR_REJECTED",
		rejectControlStatus:  "EVIDENCE_NEED_CLARIFICATION",
	})
}

// triggerAIValidation kicks off an advisory AI validation in the background.
// No-op when the AI agent client is not configured (AI_VALIDATION_ENABLED=false).
func (h *evidenceHandler) triggerAIValidation(auditID, controlID, evidenceID int, actor string) {
	triggerAIValidation(h.aiClient, auditID, controlID, evidenceID, actor)
}

// triggerAIValidation kicks off an advisory AI validation, detached from the
// request context so a client disconnect cannot cancel it. Best-effort and a
// no-op when the AI agent client is nil (AI_VALIDATION_ENABLED=false).
func triggerAIValidation(aiClient *aiagent.Client, auditID, controlID, evidenceID int, actor string) {
	if aiClient == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := aiClient.Trigger(ctx, aiagent.TriggerRequest{
			Task:        "validate_evidence",
			Scope:       aiagent.Scope{AuditID: auditID, ControlID: controlID, EvidenceID: evidenceID},
			RequestedBy: actor,
		})
		if err != nil {
			slog.Warn("ai validation trigger failed", "evidenceId", evidenceID, "err", err)
		}
	}()
}

// deleteEvidenceFile handles DELETE /api/v1/evidence/files/{fileId}.
//
// Removes a single file from an evidence submission (DB record only; the blob
// in Azure is not deleted). The caller must be the file's original uploader or
// hold ManageControls.
//
// This route carries no audit/control context, so it cannot reconcile the
// control status when the last file is removed. The web app uses the
// audit-scoped variant below; this one remains for clients that only hold a
// file id.
func (h *evidenceHandler) deleteEvidenceFile(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	fileID, ok := parseIntParam(w, r, "fileId")
	if !ok {
		return
	}
	if !h.deleteFile(w, r, fileID) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// teamEditableControlStatuses are the control statuses from which the team may
// still delete an evidence file or round — exactly the statuses where the
// frontend (DesignEvidenceSection/OEEvidenceSection) passes canDelete to
// SubmittedEvidenceList. Once a control reaches EVIDENCE_UNDER_VALIDATION,
// evidence is locked for the team; ManageControls may still act there (an
// admin correction) — but not once the control is COMPLETE, which is a hard
// lock for everyone (see requireControlNotComplete). Mirrors
// teamEditablePopulationStatuses (population.go) — this endpoint previously
// had no backend-side counterpart to that gate, relying solely on the
// frontend not rendering the delete button past this point.
var teamEditableControlStatuses = map[string]bool{
	"EVIDENCE_PENDING":            true,
	"EVIDENCE_NEED_CLARIFICATION": true,
	"EVIDENCE_INTERNAL_REVIEW":    true,
	"SUBMITTED_SAMPLE":            true,
}

// requireEditableEvidenceControl loads the control, rejects with 409 if it is
// COMPLETE (nobody may edit evidence past that point, not even
// ManageControls — see requireControlNotComplete), and otherwise requires the
// status be team-editable unless the caller holds ManageControls. Returns nil
// after writing the response on failure.
func (h *evidenceHandler) requireEditableEvidenceControl(w http.ResponseWriter, r *http.Request, auditID, controlID int) *model.AuditControl {
	control := h.requireControlNotComplete(w, r, auditID, controlID)
	if control == nil {
		return nil
	}
	if auth.HasPrivilege(r.Context(), privilege.ManageControls) {
		return control
	}
	if !teamEditableControlStatuses[control.Status] {
		response.WriteError(w, http.StatusConflict, "evidence can only be edited before validation, or after being sent back for changes")
		return nil
	}
	return control
}

// deleteControlEvidenceFile handles
// DELETE /api/v1/audits/{id}/controls/{controlId}/evidence/files/{fileId}.
//
// Same deletion as deleteEvidenceFile, but because the audit and control are in
// the path it can also keep the control status honest: a submission with no
// files left is not something a reviewer can act on, so the control drops back
// to EVIDENCE_PENDING and the submitter can upload again.
func (h *evidenceHandler) deleteControlEvidenceFile(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	fileID, ok := parseIntParam(w, r, "fileId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}
	if h.requireEditableEvidenceControl(w, r, auditID, controlID) == nil {
		return
	}
	if !h.deleteFile(w, r, fileID) {
		return
	}

	status, err := h.reconcileAfterDelete(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"status": status})
}

// deleteEvidenceRound handles
// DELETE /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}.
//
// Removes a whole fileless (attestation-only) round — the "Completed without
// files" case has no individual file to fall back on deleteControlEvidenceFile
// for. Reuses reconcileAfterDelete so a round deleted out from under an
// EVIDENCE_INTERNAL_REVIEW control drops it back to EVIDENCE_PENDING the same
// way emptying a file-based round does.
func (h *evidenceHandler) deleteEvidenceRound(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.SubmitEvidence, privilege.ManageControls) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	evidenceID, ok := parseIntParam(w, r, "evidenceId")
	if !ok {
		return
	}
	if !h.requireAssignment(w, r, auditID, controlID) {
		return
	}
	if h.requireEditableEvidenceControl(w, r, auditID, controlID) == nil {
		return
	}
	actor := auth.FromContext(r.Context()).Subject
	isAdmin := auth.HasPrivilege(r.Context(), privilege.ManageControls)
	if err := h.svc.DeleteRound(r.Context(), auditID, controlID, evidenceID, actor, isAdmin); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	status, err := h.reconcileAfterDelete(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"status": status})
}

// deleteFile performs the authorization-checked delete shared by both delete
// routes. It writes the error response and returns false on failure.
func (h *evidenceHandler) deleteFile(w http.ResponseWriter, r *http.Request, fileID int) bool {
	actor := auth.FromContext(r.Context()).Subject
	isAdmin := auth.HasPrivilege(r.Context(), privilege.ManageControls)
	if err := h.svc.DeleteFile(r.Context(), fileID, actor, isAdmin); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return false
	}
	return true
}

// reconcileAfterDelete sends a control back to EVIDENCE_PENDING when the delete
// emptied a submission that was awaiting internal review. Any other status is
// left alone: once the auditor has the evidence the submitter no longer drives
// the transition. Returns the control's status after reconciliation.
func (h *evidenceHandler) reconcileAfterDelete(ctx context.Context, auditID, controlID int) (string, error) {
	control, err := h.controlSvc.GetByID(ctx, auditID, controlID)
	if err != nil {
		return "", err
	}
	if control == nil {
		return "", nil
	}
	if control.Status != "EVIDENCE_INTERNAL_REVIEW" {
		return control.Status, nil
	}

	round, err := h.svc.LatestRound(ctx, auditID, controlID)
	if err != nil {
		// Deleting the control's only round leaves nothing for LatestRound to
		// find (a 404, not a real failure) — that's the same "no evidence
		// left" state as an empty round, not an error to surface.
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
			return "", err
		}
		round = nil
	}
	if round != nil && (len(round.Files) > 0 || round.Attestation != "") {
		return control.Status, nil
	}

	actor := auth.FromContext(ctx).Subject
	statusReq := model.UpdateStatusRequest{Status: "EVIDENCE_PENDING"}
	if err := h.controlSvc.UpdateStatus(ctx, auditID, controlID, statusReq, actor); err != nil {
		return "", err
	}
	return "EVIDENCE_PENDING", nil
}

// downloadEvidenceFile handles GET /api/v1/evidence/files/{fileId}/download.
// It proxies the file bytes from the Compliance Entity (which reads them from
// Azure) so the browser never contacts Azure directly.
// requireEvidenceFileAccess authorizes downloadEvidenceFile with the same rule
// as canViewEvidence, but resolved from a file id instead of a control — the
// download route (GET /api/v1/evidence/files/{fileId}/download) carries no
// auditId/controlId to look a control up by, so FileAuditorID returns the
// owning control's team alongside the auditor id in one round trip.
// ManageControls, SubmitEvidence, ReviewEvidence, and ViewAllAudits bypass —
// checked against that team (HasPrivilegeIn), since all four can be granted
// scoped to a single team (module=AUDIT) and the unscoped HasPrivilege would
// let such a grant download every other team's files too. Anyone else (e.g.
// an external auditor holding only ValidateEvidence) must be the
// id-matched auditor of the file's owning control.
func (h *evidenceHandler) requireEvidenceFileAccess(w http.ResponseWriter, r *http.Request, fileID int) bool {
	ctx := r.Context()
	auditorID, fileTeamID, err := h.svc.FileAuditorID(ctx, fileID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return false
	}
	teamID := 0
	if fileTeamID != nil {
		teamID = *fileTeamID
	}
	if auth.HasPrivilegeIn(ctx, privilege.ManageControls, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.SubmitEvidence, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.ReviewEvidence, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.ViewAllAudits, teamID) {
		return true
	}
	actor := auth.FromContext(ctx)
	if auditorID == nil || *auditorID != actor.UserID {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return false
	}
	return true
}

func (h *evidenceHandler) downloadEvidenceFile(w http.ResponseWriter, r *http.Request) {
	fileID, ok := parseIntParam(w, r, "fileId")
	if !ok {
		return
	}
	if !h.requireEvidenceFileAccess(w, r, fileID) {
		return
	}
	data, fileName, contentType, err := h.svc.DownloadFile(r.Context(), fileID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": fileName})
	if disposition == "" {
		disposition = `attachment; filename="file"`
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", disposition)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // #nosec G705 -- file served with nosniff + attachment disposition, browser won't execute it inline
}

// canViewEvidence allows: the team (SubmitEvidence), an internal reviewer
// (ReviewEvidence), an org-wide reader (ViewAllAudits), ManageControls, or the
// control's assigned auditor (by user id, e.g. ValidateEvidence holders). Each
// privilege is checked against control's own team (HasPrivilegeIn), since all
// four can be granted scoped to a single team (module=AUDIT) — the unscoped
// HasPrivilege would let a team-scoped grant view every other team's evidence
// too.
func canViewEvidence(r *http.Request, control *model.AuditControl) bool {
	ctx := r.Context()
	teamID := 0
	if control.TeamID != nil {
		teamID = *control.TeamID
	}
	if auth.HasPrivilegeIn(ctx, privilege.ManageControls, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.SubmitEvidence, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.ReviewEvidence, teamID) ||
		auth.HasPrivilegeIn(ctx, privilege.ViewAllAudits, teamID) {
		return true
	}
	actor := auth.FromContext(ctx)
	return control.AuditorID != nil && *control.AuditorID == actor.UserID
}

// listEvidence handles GET /api/v1/audits/{id}/controls/{controlId}/evidence.
func (h *evidenceHandler) listEvidence(w http.ResponseWriter, r *http.Request) {
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}

	control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control == nil {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	if !canViewEvidence(r, control) {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return
	}

	evidence, err := h.svc.List(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if evidence == nil {
		evidence = []*model.AuditEvidence{}
	}
	h.resolveEvidenceSubmitters(r.Context(), evidence)
	response.WriteJSONValue(w, http.StatusOK, evidence)
}

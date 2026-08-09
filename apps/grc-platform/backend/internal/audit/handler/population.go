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
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// Web-app population submission routes. These mirror the evidence submission
// flow (upload-link → upload → submit) but write POPULATION files against the
// control's active population round, then advance the control to
// POPULATION_INTERNAL_REVIEW. The Evidence Portal has its own equivalents under
// /api/v1/evidence-app (see evidence_app.go).

// activePopulationID resolves the active population round for an OE control.
// Writes 409 and returns ok=false when there is none (e.g. DESIGN control).
func (h *evidenceHandler) activePopulationID(w http.ResponseWriter, r *http.Request, controlID int) (int, bool) {
	populationID, found, err := h.controlSvc.ActivePopulationID(r.Context(), controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return 0, false
	}
	if !found {
		response.WriteError(w, http.StatusConflict, "this control has no active population phase; use the evidence endpoints")
		return 0, false
	}
	return populationID, true
}

// getPopulationUploadLink handles
// GET /api/v1/audits/{id}/controls/{controlId}/population/upload-link.
func (h *evidenceHandler) getPopulationUploadLink(w http.ResponseWriter, r *http.Request) {
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
	if _, ok := h.activePopulationID(w, r, controlID); !ok {
		return
	}
	link, err := h.svc.PopulationUploadLink(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, link)
}

// uploadPopulation handles
// POST /api/v1/audits/{id}/controls/{controlId}/population/upload.
//
// Like uploadEvidence, the client sends multipart/form-data (folderPath, file)
// and the backend proxies the bytes to Azure — no SAS reaches the client.
func (h *evidenceHandler) uploadPopulation(w http.ResponseWriter, r *http.Request) {
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
	if _, ok := h.activePopulationID(w, r, controlID); !ok {
		return
	}
	folderPath, fileName, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	if err := h.svc.ValidatePopulationFolderPath(r.Context(), auditID, controlID, folderPath); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	blobName, err := h.svc.UploadFile(r.Context(), folderPath, fileName, contentType, data)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, map[string]any{"fileName": fileName, "blobName": blobName, "size": len(data)})
}

// submitPopulation handles
// POST /api/v1/audits/{id}/controls/{controlId}/population/submit.
//
// Records every blob at folderPath as a POPULATION file on the active round and
// advances the control to POPULATION_INTERNAL_REVIEW.
func (h *evidenceHandler) submitPopulation(w http.ResponseWriter, r *http.Request) {
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
	populationID, ok := h.activePopulationID(w, r, controlID)
	if !ok {
		return
	}
	var req model.PopulationSubmitRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if err := h.svc.ValidatePopulationFolderPath(r.Context(), auditID, controlID, req.FolderPath); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	user := auth.FromContext(r.Context())
	actor := user.Email

	result, err := h.popSvc.SubmitPopulation(r.Context(), controlID, populationID, req.FolderPath, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	statusReq := model.UpdateStatusRequest{Status: "POPULATION_INTERNAL_REVIEW"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	// Best-effort audit-trail attribution: this submission came through the web app.
	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, 0, actor, channelWebApp, user.Issuer, nil)

	response.WriteJSONValue(w, http.StatusCreated, result)
}

// canViewPopulation allows: the team (SubmitEvidence), an internal reviewer
// (ReviewEvidence), an org-wide reader (ViewAllAudits, e.g. management — see
// ADR-0002), the control's assigned auditor (by email), or ManageControls.
// Unlike the write routes there is no team-assignment (IDOR) check here — this
// mirrors listEvidence/downloadEvidenceFile, which are privilege-gated only.
func canViewPopulation(r *http.Request, control *model.AuditControl) bool {
	ctx := r.Context()
	if auth.HasPrivilege(ctx, privilege.ManageControls) ||
		auth.HasPrivilege(ctx, privilege.SubmitEvidence) ||
		auth.HasPrivilege(ctx, privilege.ReviewEvidence) ||
		auth.HasPrivilege(ctx, privilege.ViewAllAudits) {
		return true
	}
	actor := auth.FromContext(ctx)
	return control.AuditorEmail != nil && strings.EqualFold(*control.AuditorEmail, actor.Email)
}

// withReadURLs computes the backend proxy download URL for each population file.
func withReadURLs(files []*model.PopulationFile) []*model.PopulationFile {
	for _, f := range files {
		url := fmt.Sprintf("/api/v1/population/files/%d/download", f.ID)
		f.ReadURL = &url
	}
	return files
}

// listPopulation handles GET /api/v1/audits/{id}/controls/{controlId}/population.
//
// Returns the control's current population round plus its files split into
// population[] (team-submitted) and sample[] (auditor-selected), and the
// auditor's sample note. A control normally has exactly one round for its whole
// lifecycle (see design doc §3.2/§8), so "current" and "latest" are the same.
func (h *evidenceHandler) listPopulation(w http.ResponseWriter, r *http.Request) {
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
	if !canViewPopulation(r, control) {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return
	}

	round, err := h.popSvc.LatestRound(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	files, err := h.popSvc.ListFiles(r.Context(), round.ID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	view := &model.PopulationView{
		Round:           round,
		PopulationFiles: []*model.PopulationFile{},
		SampleFiles:     []*model.PopulationFile{},
		SampleReference: control.SampleReference,
	}
	for _, f := range files {
		if strings.EqualFold(f.FileKind, "SAMPLE") {
			view.SampleFiles = append(view.SampleFiles, f)
		} else {
			view.PopulationFiles = append(view.PopulationFiles, f)
		}
	}
	withReadURLs(view.PopulationFiles)
	withReadURLs(view.SampleFiles)

	response.WriteJSONValue(w, http.StatusOK, view)
}

// downloadPopulationFile handles
// GET /api/v1/population/files/{fileId}/download.
// It proxies the file bytes the same way downloadEvidenceFile does — the
// backend reads the blob directly from Azure using its own storage credential.
func (h *evidenceHandler) downloadPopulationFile(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.SubmitEvidence, privilege.ReviewEvidence, privilege.ManageControls, privilege.ViewAllAudits) {
		return
	}
	fileID, ok := parseIntParam(w, r, "fileId")
	if !ok {
		return
	}
	data, fileName, contentType, err := h.popSvc.DownloadFile(r.Context(), fileID)
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

// teamEditablePopulationStatuses are the round states from which the team may
// still add/remove POPULATION-kind files — before submission, sent back from
// review, or still under internal review (mirrors EVIDENCE_INTERNAL_REVIEW,
// where the team can likewise still add/remove files up until the reviewer
// decides). It locks at COMPLIANCE_APPROVED/AUDITOR-validation stage onward.
var teamEditablePopulationStatuses = map[string]bool{
	"PENDING":             true,
	"SUBMITTED":           true,
	"COMPLIANCE_REJECTED": true,
	"AUDITOR_REJECTED":    true,
}

// deletePopulationFile handles
// DELETE /api/v1/audits/{id}/controls/{controlId}/population/files/{fileId}.
//
// Scoped to POPULATION-kind files the team is still actively editing (round not
// yet submitted, or sent back for changes). SAMPLE-kind files are editable by the
// control's assigned auditor while the round is in sampleEligibleStatuses (i.e.
// through SUBMITTED_SAMPLE — it locks once evidence review starts); ManageControls
// can always remove one for an admin correction.
func (h *evidenceHandler) deletePopulationFile(w http.ResponseWriter, r *http.Request) {
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

	file, err := h.popSvc.GetFileByID(r.Context(), fileID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	isAdmin := auth.HasPrivilege(r.Context(), privilege.ManageControls)
	if strings.EqualFold(file.FileKind, "SAMPLE") {
		if !isAdmin {
			control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
			if err != nil {
				response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
				return
			}
			if control == nil {
				response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
				return
			}
			if !requireAssignedAuditor(w, r, control, privilege.SelectSample) {
				return
			}
			if !sampleEligibleStatuses[control.Status] {
				response.WriteError(w, http.StatusConflict, "sample files can only be edited while the sample is being selected or has just been submitted")
				return
			}
			round, err := h.popSvc.LatestRound(r.Context(), auditID, controlID)
			if err != nil {
				response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
				return
			}
			if round.ID != file.PopulationID {
				response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
				return
			}
		}
	} else {
		if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
			return
		}
		if !h.requireAssignment(w, r, auditID, controlID) {
			return
		}
		if !isAdmin {
			round, err := h.popSvc.LatestRound(r.Context(), auditID, controlID)
			if err != nil {
				response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
				return
			}
			if round.ID != file.PopulationID || !teamEditablePopulationStatuses[round.Status] {
				response.WriteError(w, http.StatusConflict, "population files can only be edited before or after being sent back for changes")
				return
			}
		}
	}

	if err := h.popSvc.DeleteFile(r.Context(), fileID); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// reviewPopulation handles
// POST /api/v1/audits/{id}/controls/{controlId}/population/review.
//
// Internal reviewer decision on a submitted population: approve advances it to
// auditor validation; reject sends it back to the team on the same round
// (see design doc §3.2/§8 — both rejection paths reuse the round).
func (h *evidenceHandler) reviewPopulation(w http.ResponseWriter, r *http.Request) {
	h.decideRound(w, r, decideRoundParams{
		preGate: func(w http.ResponseWriter, r *http.Request) bool {
			return auth.RequireAnyPrivilege(r.Context(), w, privilege.ReviewEvidence, privilege.ManageControls)
		},
		requiredStatus:    "POPULATION_INTERNAL_REVIEW",
		statusConflictMsg: "population can only be reviewed while it is under internal review",
		latestRoundID: func(ctx context.Context, auditID, controlID int) (int, error) {
			round, err := h.popSvc.LatestRound(ctx, auditID, controlID)
			if err != nil {
				return 0, err
			}
			return round.ID, nil
		},
		updateRoundStatus:  h.popSvc.UpdateRoundStatus,
		approveRoundStatus: "COMPLIANCE_APPROVED",
		// Internal-review reject sends the team back to POPULATION_PENDING (the
		// same "team edits and submits" state as the first round), mirroring how
		// EVIDENCE_INTERNAL_REVIEW reject targets EVIDENCE_PENDING rather than a
		// separate clarification state. Only the auditor's validate-stage reject
		// (see validatePopulation below) uses POPULATION_NEED_CLARIFICATION.
		approveControlStatus: "POPULATION_UNDER_VALIDATION",
		rejectRoundStatus:    "COMPLIANCE_REJECTED",
		rejectControlStatus:  "POPULATION_PENDING",
		clearFilesOnReject: func(ctx context.Context, roundID int) error {
			return h.popSvc.ClearFiles(ctx, roundID, "POPULATION")
		},
	})
}

// validatePopulation handles
// POST /api/v1/audits/{id}/controls/{controlId}/population/validate.
//
// The assigned auditor's decision on a population that passed internal review:
// approve moves it to the sample phase; reject sends it back to the team on the
// same round.
func (h *evidenceHandler) validatePopulation(w http.ResponseWriter, r *http.Request) {
	h.decideRound(w, r, decideRoundParams{
		postGate:          assignedAuditorGate(privilege.ValidateEvidence),
		requiredStatus:    "POPULATION_UNDER_VALIDATION",
		statusConflictMsg: "population can only be validated while it is under auditor validation",
		latestRoundID: func(ctx context.Context, auditID, controlID int) (int, error) {
			round, err := h.popSvc.LatestRound(ctx, auditID, controlID)
			if err != nil {
				return 0, err
			}
			return round.ID, nil
		},
		updateRoundStatus:    h.popSvc.UpdateRoundStatus,
		approveRoundStatus:   "APPROVED",
		approveControlStatus: "POPULATION_COMPLETE",
		rejectRoundStatus:    "AUDITOR_REJECTED",
		rejectControlStatus:  "POPULATION_NEED_CLARIFICATION",
		clearFilesOnReject: func(ctx context.Context, roundID int) error {
			return h.popSvc.ClearFiles(ctx, roundID, "POPULATION")
		},
	})
}

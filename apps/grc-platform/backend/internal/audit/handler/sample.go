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
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// Sample-selection routes: the assigned auditor picks the sample after the
// population has passed validation. Reuses the same proxied-upload machinery as
// evidence/population (bytes flow client → backend → Azure) and the same
// population round, in a "sample/" subfolder so re-listing the round's blobs
// never conflates population and sample files.

// controlForAuditorAction fetches the control and applies the assigned-auditor
// gate in one step, writing the appropriate error response on failure.
func (h *evidenceHandler) controlForAuditorAction(w http.ResponseWriter, r *http.Request, auditID, controlID int) (*model.AuditControl, bool) {
	control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return nil, false
	}
	if control == nil {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return nil, false
	}
	if !requireAssignedAuditor(w, r, control, privilege.SelectSample) {
		return nil, false
	}
	return control, true
}

// sampleEligibleStatuses are the control states from which the auditor may pick,
// submit, or edit a sample. SUBMITTED_SAMPLE is included so the auditor can still
// add/remove sample files and update the note right after submitting — the round
// locks once evidence work has moved past that point (EVIDENCE_INTERNAL_REVIEW
// onward), matching how population stays editable through the review-reject
// round-trip but not indefinitely.
var sampleEligibleStatuses = map[string]bool{
	"POPULATION_COMPLETE": true,
	"AWAITING_SAMPLE":     true,
	"SUBMITTED_SAMPLE":    true,
}

// getSampleUploadLink handles
// GET /api/v1/audits/{id}/controls/{controlId}/sample/upload-link.
func (h *evidenceHandler) getSampleUploadLink(w http.ResponseWriter, r *http.Request) {
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	control, ok := h.controlForAuditorAction(w, r, auditID, controlID)
	if !ok {
		return
	}
	if !sampleEligibleStatuses[control.Status] {
		response.WriteError(w, http.StatusConflict, "sample can only be selected once the population has been approved")
		return
	}
	link, err := h.svc.SampleUploadLink(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, link)
}

// uploadSample handles POST /api/v1/audits/{id}/controls/{controlId}/sample/upload.
// Multipart upload, proxied to Azure exactly like uploadPopulation/uploadEvidence.
func (h *evidenceHandler) uploadSample(w http.ResponseWriter, r *http.Request) {
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	control, ok := h.controlForAuditorAction(w, r, auditID, controlID)
	if !ok {
		return
	}
	if !sampleEligibleStatuses[control.Status] {
		response.WriteError(w, http.StatusConflict, "sample can only be selected once the population has been approved")
		return
	}
	folderPath, fileName, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	if err := h.svc.ValidateSampleFolderPath(r.Context(), auditID, controlID, folderPath); err != nil {
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

// submitSample handles POST /api/v1/audits/{id}/controls/{controlId}/sample/submit.
//
// Records every blob at folderPath as a SAMPLE file, sets the control's sample
// note, and advances it to SUBMITTED_SAMPLE — the entry point to the evidence
// phase for the team.
func (h *evidenceHandler) submitSample(w http.ResponseWriter, r *http.Request) {
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	control, ok := h.controlForAuditorAction(w, r, auditID, controlID)
	if !ok {
		return
	}
	if !sampleEligibleStatuses[control.Status] {
		response.WriteError(w, http.StatusConflict, "sample can only be selected once the population has been approved")
		return
	}
	round, err := h.popSvc.LatestRound(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	var req model.SampleSubmitRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if err := h.svc.ValidateSampleFolderPath(r.Context(), auditID, controlID, req.FolderPath); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	user := auth.FromContext(r.Context())
	actor := user.Email
	fileCount, err := h.popSvc.SubmitSample(r.Context(), round.ID, req.FolderPath, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// Files and the note are each optional, but at least one is required — a
	// sample with neither has nothing for the team to act on.
	if fileCount == 0 && strings.TrimSpace(req.Note) == "" {
		response.WriteError(w, http.StatusBadRequest, "provide sample files, a note, or both")
		return
	}
	if err := h.controlSvc.UpdateStatusWithSample(r.Context(), auditID, controlID, "SUBMITTED_SAMPLE", req.Note, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// UpdateStatusWithSample already records a generic status-change trail row
	// (statusChangeAction); this adds the same explicit attribution row that
	// evidence/population submission get, matching population.go's submitPopulation.
	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, 0, actor, channelWebApp, user.Issuer, nil)
	response.WriteJSONValue(w, http.StatusCreated, map[string]any{
		"status":    "SUBMITTED_SAMPLE",
		"fileCount": fileCount,
	})
}

// requestSampleTime handles
// POST /api/v1/audits/{id}/controls/{controlId}/sample/request-time.
//
// Lets the auditor signal they need more time before selecting a sample:
// POPULATION_COMPLETE → AWAITING_SAMPLE. A plain status flip, no note field —
// the team sees a generic "auditor is preparing the sample" message.
func (h *evidenceHandler) requestSampleTime(w http.ResponseWriter, r *http.Request) {
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	control, ok := h.controlForAuditorAction(w, r, auditID, controlID)
	if !ok {
		return
	}
	if control.Status != "POPULATION_COMPLETE" {
		response.WriteError(w, http.StatusConflict, "more time can only be requested right after the population is approved")
		return
	}
	actor := auth.FromContext(r.Context()).Email
	statusReq := model.UpdateStatusRequest{Status: "AWAITING_SAMPLE"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, map[string]any{"status": "AWAITING_SAMPLE"})
}

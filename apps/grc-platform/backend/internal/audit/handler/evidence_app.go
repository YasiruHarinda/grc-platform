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
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/aiagent"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// Channel tags distinguish who performed an action in the audit trail (§I).
const (
	channelWebApp      = "web-app"
	channelEvidenceApp = "evidence-app"
)

// phaseFor returns the submission phase for a control status: POPULATION for the
// population phase (status prefix "POPULATION_"), EVIDENCE otherwise.
func phaseFor(status string) string {
	if strings.HasPrefix(status, "POPULATION_") {
		return "POPULATION"
	}
	return "EVIDENCE"
}

// baseFolderPathFor returns the phase-aware control-level Azure Blob prefix,
// built from the audit name and control number (never trusted from a client —
// both come from the server-side control lookup). Matches the layout
// EvidenceService.GetUploadLink/PopulationUploadLink derive for the same
// control, so the Evidence Portal's displayed path always agrees with reality.
func baseFolderPathFor(phase, auditName, controlNumber string) string {
	sub := "evidence"
	if phase == "POPULATION" {
		sub = "population"
	}
	return fmt.Sprintf("%s/%s/%s/", service.SanitizeSegment(auditName), service.SanitizeSegment(controlNumber), sub)
}

// recordEvidenceTrail appends a best-effort attribution entry. Failures are logged
// and swallowed — they never affect the submission the user just made.
func recordEvidenceTrail(ctx context.Context, trailSvc service.TrailService, auditID, controlID, evidenceID int, actor, via, issuer string) {
	if trailSvc == nil {
		return
	}
	if err := trailSvc.RecordEvidenceAction(ctx, auditID, controlID, evidenceID, "UPLOADED", actor, via, issuer); err != nil {
		slog.WarnContext(ctx, "audit-trail attribution failed", "controlId", controlID, "via", via, "err", err)
	}
}

// evidenceAppHandler serves the Evidence Portal proxy API (/api/v1/evidence-app/*).
// It is callable by IdP-2 (portal) tokens and by IdP-1 users holding SUBMIT_EVIDENCE.
// Handlers are thin: privilege check → resource assignment check (which also yields
// the server-derived auditID) → folder-path binding → delegate to the same services
// the web-app handlers use.
type evidenceAppHandler struct {
	svc        service.EvidenceService
	controlSvc service.ControlService
	popSvc     service.PopulationService
	trailSvc   service.TrailService
	aiClient   *aiagent.Client
}

// assignedAuditID confirms the caller is assigned to controlID for an actionable
// status and returns the server-derived audit id. Writes 403 and returns ok=false
// when not assigned.
func (h *evidenceAppHandler) assignedAuditID(w http.ResponseWriter, r *http.Request, controlID int) (int, bool) {
	actor := auth.FromContext(r.Context())
	auditID, found, err := h.controlSvc.AssignedAuditID(r.Context(), actor.Email, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return 0, false
	}
	if !found {
		response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
		return 0, false
	}
	return auditID, true
}

// activePopulationID resolves the active population round for an OE control. Writes
// 409 and returns ok=false when there is none (e.g. the control is DESIGN-type).
func (h *evidenceAppHandler) activePopulationID(w http.ResponseWriter, r *http.Request, controlID int) (int, bool) {
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

// listControls handles GET /api/v1/evidence-app/controls (design §3.1). Returns
// every actionable control for the caller's team, enriched with audit/product/
// framework and a computed phase + phase-aware base folder path.
func (h *evidenceAppHandler) listControls(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	actor := auth.FromContext(r.Context())
	controls, err := h.controlSvc.GetAssignedForEvidence(r.Context(), actor.Email)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	out := make([]model.EvidenceAppControl, 0, len(controls))
	for _, c := range controls {
		phase := phaseFor(c.Status)
		out = append(out, model.EvidenceAppControl{
			Audit: model.EvidenceAppAudit{
				ID:          c.AuditID,
				Name:        c.AuditName,
				Product:     c.Product,
				Framework:   c.Framework,
				PeriodStart: c.PeriodStart,
				PeriodEnd:   c.PeriodEnd,
			},
			Control: model.EvidenceAppControlInfo{
				ID:                  c.ControlID,
				Number:              c.ControlNumber,
				Description:         c.Description,
				EvidenceRequirement: c.EvidenceRequirement,
				RequirementType:     c.RequirementType,
				Status:              c.Status,
				Phase:               phase,
				DueDate:             c.DueDate,
			},
			BaseFolderPath: baseFolderPathFor(phase, c.AuditName, c.ControlNumber),
		})
	}
	response.WriteJSONValue(w, http.StatusOK, out)
}

// uploadLink handles GET /api/v1/evidence-app/controls/{controlId}/upload-link (§3.2).
func (h *evidenceAppHandler) uploadLink(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	auditID, ok := h.assignedAuditID(w, r, controlID)
	if !ok {
		return
	}
	link, err := h.svc.GetUploadLink(r.Context(), auditID, controlID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, link)
}

// upload handles POST /api/v1/evidence-app/controls/{controlId}/upload (§3.3).
func (h *evidenceAppHandler) upload(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	auditID, ok := h.assignedAuditID(w, r, controlID)
	if !ok {
		return
	}
	folderPath, fileName, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	if err := h.svc.ValidateEvidenceFolderPath(r.Context(), auditID, controlID, folderPath); err != nil {
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

// submit handles POST /api/v1/evidence-app/controls/{controlId}/submit (§3.4).
func (h *evidenceAppHandler) submit(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	auditID, ok := h.assignedAuditID(w, r, controlID)
	if !ok {
		return
	}
	var req model.SubmitEvidenceRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	user := auth.FromContext(r.Context())
	actor := user.Email

	evidence, err := h.svc.Submit(r.Context(), auditID, controlID, req.Files, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	statusReq := model.UpdateStatusRequest{Status: "EVIDENCE_INTERNAL_REVIEW"}
	if err := h.controlSvc.UpdateStatus(r.Context(), auditID, controlID, statusReq, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, evidence.ID, actor, channelEvidenceApp, user.Issuer)
	triggerAIValidation(h.aiClient, auditID, controlID, evidence.ID, actor)

	response.WriteJSONValue(w, http.StatusCreated, evidence)
}

// populationUploadLink handles
// GET /api/v1/evidence-app/controls/{controlId}/population/upload-link (§3.5.1).
func (h *evidenceAppHandler) populationUploadLink(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	auditID, ok := h.assignedAuditID(w, r, controlID)
	if !ok {
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

// populationUpload handles
// POST /api/v1/evidence-app/controls/{controlId}/population/upload (§3.5.2).
func (h *evidenceAppHandler) populationUpload(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	auditID, ok := h.assignedAuditID(w, r, controlID)
	if !ok {
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

// populationSubmit handles
// POST /api/v1/evidence-app/controls/{controlId}/population/submit (§3.5.3).
func (h *evidenceAppHandler) populationSubmit(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.SubmitEvidence) {
		return
	}
	controlID, ok := parseIntParam(w, r, "controlId")
	if !ok {
		return
	}
	auditID, ok := h.assignedAuditID(w, r, controlID)
	if !ok {
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

	recordEvidenceTrail(r.Context(), h.trailSvc, auditID, controlID, 0, actor, channelEvidenceApp, user.Issuer)

	response.WriteJSONValue(w, http.StatusCreated, result)
}

// readUpload parses a bounded multipart upload (folderPath + file), returning the
// folder path, base file name, sniffed content type, and bytes. It writes the error
// response and returns ok=false on any failure.
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

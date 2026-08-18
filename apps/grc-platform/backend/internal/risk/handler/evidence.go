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
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	riskservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// maxRiskEvidenceUploadBytes caps a single proxied evidence upload (bytes
// travel through the backend, so bound them to protect memory and the
// gateway). Matches riskservice.maxRiskEvidenceBytes.
const maxRiskEvidenceUploadBytes = 25 << 20 // 25 MiB

// handleUploadRiskEvidence serves POST /api/v1/risks/{id}/evidence.
//
// The client sends the file as multipart/form-data (fields: file,
// evidenceType, actionPlanId (optional), note (optional)). Bytes travel
// browser -> backend -> Compliance Entity -> Azure; no SAS is ever handed to
// the client.
//
// evidenceType gates who may call this:
//   - ACTION_PLAN_ATTACHMENT ("Risk Evidence Attachment"): the Add Risk
//     form, right after the risk itself is created — requires CreateRisk
//     scoped to the risk's source register, and being the risk's assigner
//     (or the compliance-admin override), via requireRiskAssigner.
//   - FINAL_APPROVAL_ATTACHMENT ("Risk Action Plan Completion Attachment"):
//     the action owner, before "Complete Action Plan" — identity-only, no
//     privilege check, the same gate handleCompleteActionPlan uses.
func (d *Deps) handleUploadRiskEvidence(w http.ResponseWriter, r *http.Request) {
	by, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRiskEvidenceUploadBytes)
	if err := r.ParseMultipartForm(maxRiskEvidenceUploadBytes); err != nil { // #nosec G120 -- body already bounded by MaxBytesReader above
		response.WriteError(w, http.StatusRequestEntityTooLarge, "file too large or malformed upload (max 25 MB)")
		return
	}

	evidenceType := strings.ToUpper(strings.TrimSpace(r.FormValue("evidenceType")))
	var actionPlanID *int
	if raw := r.FormValue("actionPlanId"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			response.WriteError(w, http.StatusBadRequest, "actionPlanId must be a positive integer")
			return
		}
		actionPlanID = &id
	}

	switch evidenceType {
	case riskservice.EvidenceTypeActionPlanAttachment:
		// Matches handleCreateActionPlan's gate — this upload only ever
		// happens right after the risk itself is created, via the Add Risk
		// form, so the only legitimate caller is the risk's own assigner (or
		// the compliance-admin override), not merely anyone who can view it.
		if !d.requireRiskAssigner(w, r, riskID, privilege.CreateRisk) {
			return
		}
	case riskservice.EvidenceTypeFinalApprovalAttachment:
		if actionPlanID == nil {
			response.WriteError(w, http.StatusBadRequest, "actionPlanId is required for FINAL_APPROVAL_ATTACHMENT")
			return
		}
		// Identity gate: only the plan's own action owner (or the
		// compliance-admin override) may attach its completion evidence —
		// GetByID(riskID, planID) also 404s if the plan isn't this risk's.
		// Deliberately no privilege check: RISK_COMPLETE_ACTION_STEPS was
		// retired with the action-owner role (see handleUpdateActionPlanStep),
		// because an Action Owner may be any employee and hold no role at all.
		plan, err := d.ActionPlan.GetByID(r.Context(), riskID, *actionPlanID)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		registerID, err := d.sourceRegisterOf(r.Context(), riskID)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if !canOverrideAssigneeIn(r.Context(), registerID) {
			callerID, err := d.callerUserID(r.Context())
			if err != nil {
				response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
				return
			}
			if callerID == nil || plan.ActionOwnerID == nil || *plan.ActionOwnerID != *callerID {
				response.WriteError(w, http.StatusForbidden, response.ErrMsgForbidden)
				return
			}
		}
	default:
		response.WriteError(w, http.StatusBadRequest, "evidenceType must be ACTION_PLAN_ATTACHMENT or FINAL_APPROVAL_ATTACHMENT")
		return
	}

	f, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()

	fileName := filepath.Base(header.Filename)
	contentType := header.Header.Get("Content-Type")
	note := r.FormValue("note")

	ev, err := d.Evidence.Upload(r.Context(), riskID, evidenceType, actionPlanID, fileName, contentType, io.LimitReader(f, maxRiskEvidenceUploadBytes+1), note, by)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusCreated, ev)
}

// handleListRiskEvidence serves GET /api/v1/risks/{id}/evidence.
func (d *Deps) handleListRiskEvidence(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	visible, err := d.riskVisibleToCaller(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !visible {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	evidence, err := d.Evidence.List(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if evidence == nil {
		evidence = []*model.RiskEvidence{}
	}
	response.WriteJSONValue(w, http.StatusOK, evidence)
}

// handleDeleteRiskEvidence serves DELETE /api/v1/risks/{id}/evidence/{fileId}.
// The caller must be the file's original uploader or hold the compliance-admin
// override. The blob in Azure is not deleted — only the DB record is removed.
func (d *Deps) handleDeleteRiskEvidence(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserEmail(w, r)
	if !ok {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil || fileID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "fileId must be a positive integer")
		return
	}
	registerID, err := d.sourceRegisterOf(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if err := d.Evidence.Delete(r.Context(), riskID, fileID, actor, canOverrideAssigneeIn(r.Context(), registerID)); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDownloadRiskEvidence serves
// GET /api/v1/risks/{id}/evidence/{fileId}/download. Proxies the file bytes
// from the Compliance Entity (which reads them from Azure) so the browser
// never contacts Azure directly.
func (d *Deps) handleDownloadRiskEvidence(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewRisks) {
		return
	}
	riskID, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	visible, err := d.riskVisibleToCaller(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !visible {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}
	fileID, err := strconv.Atoi(r.PathValue("fileId"))
	if err != nil || fileID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "fileId must be a positive integer")
		return
	}
	data, fileName, contentType, err := d.Evidence.DownloadFile(r.Context(), riskID, fileID)
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

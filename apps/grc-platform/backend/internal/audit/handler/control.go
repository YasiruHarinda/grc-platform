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
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type controlHandler struct {
	svc service.ControlService
	// notify sends owner-assignment notification emails after addControl/
	// bulkAddControls/updateControl — see notify.go.
	notify *Deps
}

// listControls handles GET /api/v1/audits/{id}/controls.
func (h *controlHandler) listControls(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	ctx := r.Context()
	scope, _ := deriveScopes(ctx)
	user := auth.FromContext(ctx)
	var email string
	if user != nil {
		email = user.Email
	}
	controls, err := h.svc.ListScoped(ctx, auditID, scope, email, managedTeamIDs(auth.Grants(ctx)))
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	if controls == nil {
		controls = []*model.AuditControl{}
	}
	response.WriteJSONValue(w, http.StatusOK, &model.ControlListResponse{
		Items: controls,
		Total: len(controls),
	})
}

// getControl handles GET /api/v1/audits/{id}/controls/{controlId}.
func (h *controlHandler) getControl(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
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
	ctx := r.Context()
	scope, _ := deriveScopes(ctx)
	if scope != model.ScopeAll {
		user := auth.FromContext(ctx)
		var email string
		if user != nil {
			email = user.Email
		}
		inScope, err := h.svc.InScope(ctx, auditID, controlID, scope, email, managedTeamIDs(auth.Grants(ctx)))
		if err != nil {
			response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
			return
		}
		if !inScope {
			response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
			return
		}
	}
	c, err := h.svc.GetByID(ctx, auditID, controlID)
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return
	}
	response.WriteJSONValue(w, http.StatusOK, c)
}

// addControl handles POST /api/v1/audits/{id}/controls.
func (h *controlHandler) addControl(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageControls) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	var req model.AddControlRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	c, err := h.svc.Add(r.Context(), auditID, req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	h.notifyOwnerAssignments(r.Context(), auditID, []model.AddControlRequest{req}, []*model.AuditControl{c}, actor)
	response.WriteJSONValue(w, http.StatusCreated, c)
}

// maxBulkBodyBytes caps the bulk-add request body. It is larger than the default
// 1 MiB so a full maxBulkControls (500) import with paragraph-length descriptions
// fits — roughly 8 KB per control — and clients hit the friendly 422 item-count
// error rather than a generic 413 first.
const maxBulkBodyBytes = 4 << 20 // 4 MiB

// bulkAddControls handles POST /api/v1/audits/{id}/controls/bulk.
// Used by the Create Audit form when copying from a previous audit or uploading CSV.
func (h *controlHandler) bulkAddControls(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageControls) {
		return
	}
	auditID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}
	var req model.BulkAddControlsRequest
	if err := response.DecodeJSONLimit(w, r, &req, maxBulkBodyBytes); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	controls, err := h.svc.BulkAdd(r.Context(), auditID, req.Controls, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	h.notifyOwnerAssignments(r.Context(), auditID, req.Controls, controls, actor)
	response.WriteJSONValue(w, http.StatusCreated, &model.ControlListResponse{
		Items: controls,
		Total: len(controls),
	})
}

// updateControl handles PUT /api/v1/audits/{id}/controls/{controlId}.
func (h *controlHandler) updateControl(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageControls) {
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
	var req model.UpdateControlRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	result, err := h.svc.Update(r.Context(), auditID, controlID, req, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	h.notifyReassignments(r.Context(), auditID, controlID, result, actor)
	w.WriteHeader(http.StatusNoContent)
}

// recipientBatch accumulates one AuditEventItem batch per recipient user ID
// for one AuditEvent, alongside which control(s) that recipient's batch
// touches (for the deep-link decision, see singleControlID) and the
// audit_notification log rows to write per recipient. Shared shape between
// the owner and auditor batches notifyOwnerAssignments/notifyReassignments
// build — same event kind (someone assigned to a control), different role.
type recipientBatch struct {
	items      map[int][]emailer.AuditEventItem
	logs       map[int][]notificationLogItem
	controlIDs map[int]map[int]bool
}

func newRecipientBatch() *recipientBatch {
	return &recipientBatch{
		items:      map[int][]emailer.AuditEventItem{},
		logs:       map[int][]notificationLogItem{},
		controlIDs: map[int]map[int]bool{},
	}
}

func (b *recipientBatch) add(recipientID, controlID int, item emailer.AuditEventItem, logItem notificationLogItem) {
	b.items[recipientID] = append(b.items[recipientID], item)
	b.logs[recipientID] = append(b.logs[recipientID], logItem)
	if b.controlIDs[recipientID] == nil {
		b.controlIDs[recipientID] = map[int]bool{}
	}
	b.controlIDs[recipientID][controlID] = true
}

// sendBatched fires ev once per recipient in batch — one email per person,
// covering every item batched for them. Deep-links straight to a control
// when the recipient's batch is about only one; falls back to the audit's
// control list when they picked up more than one control in the same
// submission (see singleControlID).
func (h *controlHandler) sendBatched(ctx context.Context, ev emailer.AuditEvent, auditID int, auditName, actor string, batch *recipientBatch) {
	for recipientID, items := range batch.items {
		detailURL := h.notify.detailURL(auditID)
		if controlID, ok := singleControlID(batch.controlIDs[recipientID]); ok {
			detailURL = h.notify.controlDetailURL(auditID, controlID)
		}
		info := emailer.AuditEventInfo{
			AuditName: auditName,
			Actor:     h.notify.describeActor(ctx, actor),
			DetailURL: detailURL,
			Items:     items,
		}
		h.notify.notifyAuditEvent(ev, recipientID, info, batch.logs[recipientID])
	}
}

// notifyOwnerAssignments batches every control's control-owner AND
// population-owner in reqs into one AuditEventOwnerAssigned email per
// distinct owner — so a person who is both a control's and its population's
// owner in the same request gets exactly one email, not two (see
// emailer.AuditEventOwnerAssigned) — and separately batches every assigned
// auditor into one AuditEventAuditorAssigned email per distinct auditor.
// Owner and auditor are deliberately separate emails even when the same
// person holds both roles across the batch: they're different kinds of
// assignment. controls is matched to reqs by ControlNumber (unique per
// audit) rather than index, since bulkAddControls's response is sorted by
// control number, not request order.
func (h *controlHandler) notifyOwnerAssignments(ctx context.Context, auditID int, reqs []model.AddControlRequest, controls []*model.AuditControl, actor string) {
	byNumber := make(map[string]*model.AuditControl, len(controls))
	for _, c := range controls {
		if c != nil {
			byNumber[c.ControlNumber] = c
		}
	}

	owners := newRecipientBatch()
	auditors := newRecipientBatch()

	for _, req := range reqs {
		c, ok := byNumber[req.ControlNumber]
		if !ok {
			continue
		}
		if req.OwnerID != nil {
			owners.add(*req.OwnerID, c.ID, emailer.AuditEventItem{
				ControlNumber:   c.ControlNumber,
				Description:     c.Description,
				DueDate:         derefString(c.DueDate),
				RequirementType: "Evidence Requirement",
			}, notificationLogItem{
				AuditID:   &auditID,
				Type:      "OWNER_ASSIGNED_CONTROL",
				ControlID: &c.ID,
			})
		}
		if req.Population != nil && req.Population.OwnerID != nil {
			owners.add(*req.Population.OwnerID, c.ID, emailer.AuditEventItem{
				ControlNumber:   c.ControlNumber,
				Description:     c.Description,
				DueDate:         derefString(req.Population.DueDate),
				RequirementType: "Population Requirement",
			}, notificationLogItem{
				AuditID:      &auditID,
				Type:         "OWNER_ASSIGNED_POPULATION",
				PopulationID: c.PopulationID,
			})
		}
		if req.AuditorID != nil {
			auditors.add(*req.AuditorID, c.ID, emailer.AuditEventItem{
				ControlNumber:   c.ControlNumber,
				Description:     c.Description,
				DueDate:         derefString(c.DueDate),
				RequirementType: "Evidence Requirement",
			}, notificationLogItem{
				AuditID:   &auditID,
				Type:      "AUDITOR_ASSIGNED_CONTROL",
				ControlID: &c.ID,
			})
		}
	}

	name := h.notify.auditName(ctx, auditID)
	h.sendBatched(ctx, emailer.AuditEventOwnerAssigned, auditID, name, actor, owners)
	h.sendBatched(ctx, emailer.AuditEventAuditorAssigned, auditID, name, actor, auditors)
}

// notifyReassignments fires the owner/auditor-assigned events for
// updateControl's reassignments — new control owner, new population owner,
// and/or new auditor — coalesced into one owner-assigned email if the first
// two land on the same person, same as notifyOwnerAssignments. Update
// doesn't return the updated control (only what changed), so this re-fetches
// it once, only when there's actually a reassignment to notify about.
func (h *controlHandler) notifyReassignments(ctx context.Context, auditID, controlID int, result service.ControlUpdateResult, actor string) {
	if !result.ControlOwnerChanged && !result.PopulationOwnerChanged && !result.AuditorChanged {
		return
	}
	c, err := h.svc.GetByID(ctx, auditID, controlID)
	if err != nil || c == nil {
		return
	}

	owners := newRecipientBatch()
	auditors := newRecipientBatch()
	if result.ControlOwnerChanged && result.NewControlOwnerID != nil {
		owners.add(*result.NewControlOwnerID, c.ID, emailer.AuditEventItem{
			ControlNumber:   c.ControlNumber,
			Description:     c.Description,
			DueDate:         derefString(c.DueDate),
			RequirementType: "Evidence Requirement",
		}, notificationLogItem{
			AuditID:   &auditID,
			Type:      "OWNER_ASSIGNED_CONTROL",
			ControlID: &c.ID,
		})
	}
	if result.PopulationOwnerChanged && result.NewPopulationOwnerID != nil {
		owners.add(*result.NewPopulationOwnerID, c.ID, emailer.AuditEventItem{
			ControlNumber:   c.ControlNumber,
			Description:     c.Description,
			DueDate:         derefString(c.PopulationDueDate),
			RequirementType: "Population Requirement",
		}, notificationLogItem{
			AuditID:      &auditID,
			Type:         "OWNER_ASSIGNED_POPULATION",
			PopulationID: c.PopulationID,
		})
	}
	if result.AuditorChanged && result.NewAuditorID != nil {
		auditors.add(*result.NewAuditorID, c.ID, emailer.AuditEventItem{
			ControlNumber:   c.ControlNumber,
			Description:     c.Description,
			DueDate:         derefString(c.DueDate),
			RequirementType: "Evidence Requirement",
		}, notificationLogItem{
			AuditID:   &auditID,
			Type:      "AUDITOR_ASSIGNED_CONTROL",
			ControlID: &c.ID,
		})
	}

	name := h.notify.auditName(ctx, auditID)
	h.sendBatched(ctx, emailer.AuditEventOwnerAssigned, auditID, name, actor, owners)
	h.sendBatched(ctx, emailer.AuditEventAuditorAssigned, auditID, name, actor, auditors)
}

// derefString returns "" for a nil pointer instead of dereferencing it.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// updateControlStatus handles PATCH /api/v1/audits/{id}/controls/{controlId}/status.
func (h *controlHandler) updateControlStatus(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageControls) {
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
	var req model.UpdateStatusRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	if err := h.svc.UpdateStatus(r.Context(), auditID, controlID, req, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// overrideControlStatus handles POST /api/v1/audits/{id}/controls/{controlId}/status/override.
//
// Gated on the unscoped ManageControls, like every other control-CRUD write in
// this file (addControl/updateControl/deleteControl/updateControlStatus) —
// unlike evidence/population's HasPrivilegeIn bypass checks, ManageControls
// itself is never granted scoped to a single team, so there is no narrower
// scope to check here.
func (h *controlHandler) overrideControlStatus(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageControls) {
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
	var req model.OverrideStatusRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	actor := auth.FromContext(r.Context()).Email
	if err := h.svc.OverrideStatus(r.Context(), auditID, controlID, req, actor); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control, err := h.svc.GetByID(r.Context(), auditID, controlID); err == nil && control != nil {
		h.notify.notifyControlStatusReached(r.Context(), control, req.Status, actor)
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteControl handles DELETE /api/v1/audits/{id}/controls/{controlId}.
func (h *controlHandler) deleteControl(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageControls) {
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
	deletedBy := auth.FromContext(r.Context()).Email
	if err := h.svc.Delete(r.Context(), auditID, controlID, deletedBy); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

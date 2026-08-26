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
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type commentHandler struct {
	svc        service.CommentService
	controlSvc service.ControlService
	// notify sends the comment-added notification email — see notify.go.
	notify    *Deps
	directory *directory.Service
}

// resolveCommentAuthor fills c.CreatedByName from c.CreatedBy (the
// commenter's raw uuid), routed to the right identity org via
// c.CreatedByUserType — see AuditTrailEntry.CreatedByName for the same
// pattern batched across a list.
func (h *commentHandler) resolveCommentAuthor(ctx context.Context, c *model.AuditComment) {
	if c == nil || c.CreatedBy == "" {
		return
	}
	p, found := h.directory.LookupTyped(ctx, c.CreatedBy, c.CreatedByUserType)
	switch {
	case found && strings.TrimSpace(p.DisplayName) != "":
		c.CreatedByName = strings.TrimSpace(p.DisplayName)
	case found && p.Email != "":
		c.CreatedByName = p.Email
	default:
		c.CreatedByName = c.CreatedBy
	}
}

// resolveCommentAuthors batch-resolves CreatedByName for a list of comments —
// see resolveCommentAuthor.
func (h *commentHandler) resolveCommentAuthors(ctx context.Context, comments []*model.AuditComment) {
	uuidTypes := make(map[string]string, len(comments))
	for _, c := range comments {
		if c.CreatedBy != "" {
			uuidTypes[c.CreatedBy] = c.CreatedByUserType
		}
	}
	people := h.directory.LookupAllTyped(ctx, uuidTypes)
	for _, c := range comments {
		p, ok := people[c.CreatedBy]
		if !ok {
			c.CreatedByName = c.CreatedBy
			continue
		}
		switch {
		case strings.TrimSpace(p.DisplayName) != "":
			c.CreatedByName = strings.TrimSpace(p.DisplayName)
		case p.Email != "":
			c.CreatedByName = p.Email
		default:
			c.CreatedByName = c.CreatedBy
		}
	}
}

// controlInScope enforces the same row-level visibility as getControl
// (handler/control.go) for the control a comment thread belongs to: without
// this, ViewAudits/AddComment being unscoped privileges meant any holder —
// including an external auditor scoped to a single control via
// ScopeAssigned — could read or post comments on any control in any audit.
// Writes a 404 (never 403, to avoid confirming the control exists in a
// different scope) and returns false on failure.
func (h *commentHandler) controlInScope(w http.ResponseWriter, r *http.Request, auditID, controlID int) bool {
	ctx := r.Context()
	scope, _ := deriveScopes(ctx)
	if scope == model.ScopeAll {
		return true
	}
	user := auth.FromContext(ctx)
	var userID int
	if user != nil {
		userID = user.UserID
	}
	inScope, err := h.controlSvc.InScope(ctx, auditID, controlID, scope, userID, managedTeamIDs(auth.Grants(ctx)))
	if err != nil {
		response.MapServiceError(ctx, w, err, response.ErrMsgInternal)
		return false
	}
	if !inScope {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return false
	}
	return true
}

// listComments handles GET /api/v1/audits/{id}/controls/{controlId}/comments.
func (h *commentHandler) listComments(w http.ResponseWriter, r *http.Request) {
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
	if !h.controlInScope(w, r, auditID, controlID) {
		return
	}
	// Internal-only comments are shown to holders of the internal-comments
	// privilege (the internal roles) and hidden from external auditors, who
	// lack it.
	includeInternal := auth.HasPrivilege(r.Context(), privilege.ViewInternalComments)
	comments, err := h.svc.List(r.Context(), auditID, controlID, includeInternal)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if comments == nil {
		comments = []*model.AuditComment{}
	}
	h.resolveCommentAuthors(r.Context(), comments)
	response.WriteJSONValue(w, http.StatusOK, &model.CommentListResponse{Items: comments})
}

// addComment handles POST /api/v1/audits/{id}/controls/{controlId}/comments.
func (h *commentHandler) addComment(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.AddComment) {
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
	if !h.controlInScope(w, r, auditID, controlID) {
		return
	}
	var req model.AddCommentRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}
	caller := auth.FromContext(r.Context())
	actor := caller.Subject
	// IsInternal is derived server-side from the caller's own privilege, never
	// trusted from the request body — otherwise a caller with AddComment but
	// not ViewInternalComments (e.g. an external auditor) could mark their own
	// comment internal.
	isInternal := req.IsInternal && auth.HasPrivilege(r.Context(), privilege.ViewInternalComments)
	c, err := h.svc.Add(r.Context(), auditID, controlID, req, isInternal, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if control, err := h.controlSvc.GetByID(r.Context(), auditID, controlID); err == nil && control != nil {
		h.notify.notifyCommentAdded(r.Context(), control, c, actor, caller.UserID)
	}
	h.resolveCommentAuthor(r.Context(), c)
	response.WriteJSONValue(w, http.StatusCreated, c)
}

// deleteComment handles
// DELETE /api/v1/audits/{id}/controls/{controlId}/comments/{commentId}.
//
// The caller must be the comment's original author or hold ManageControls.
// isAdmin below is intentionally the unscoped HasPrivilege: ManageControls is
// never granted scoped to a single team (enforced by the entity's
// grantService.validateScope, not just convention), so there is no team to
// check it against (contrast evidence/population's HasPrivilegeIn bypass
// checks, which exist because those privileges can be team-scoped).
func (h *commentHandler) deleteComment(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.AddComment) {
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
	commentID, ok := parseIntParam(w, r, "commentId")
	if !ok {
		return
	}
	actor := auth.FromContext(r.Context()).Subject
	isAdmin := auth.HasPrivilege(r.Context(), privilege.ManageControls)
	if err := h.svc.Delete(r.Context(), auditID, controlID, commentID, actor, isAdmin); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

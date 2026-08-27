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
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	auditservice "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/service"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

type dashboardHandler struct {
	svc auditservice.DashboardService
}

// deriveScopes computes the actor's view and work-queue row scopes from their
// grants — no role name is consulted.
func deriveScopes(ctx context.Context) (view, workQueue model.Scope) {
	if auth.AllowAll(ctx) { // local dev, MUST be first — auth.Grants(ctx) is nil here
		return model.ScopeAll, model.ScopeAll
	}
	set := auth.Grants(ctx)
	switch {
	case set.HasGlobal(privilege.ViewAllAudits):
		return model.ScopeAll, model.ScopeAll
	case len(managedTeamIDs(set)) > 0: // inert until a team grant exists
		return model.ScopeTeam, model.ScopeTeam
	case auth.HasPrivilege(ctx, privilege.SubmitEvidence):
		return model.ScopeOwned, model.ScopeOwned
	case auth.HasPrivilege(ctx, privilege.ValidateEvidence):
		return model.ScopeAssigned, model.ScopeAssigned
	default:
		return model.ScopeNone, model.ScopeNone
	}
}

// managedTeamIDs returns the teams where the caller holds org-wide read
// NON-globally — i.e. AUDIT_VIEW_ALL_AUDITS scoped to a team, the only
// team-lead shape this module has. Safe to call HasIn here: it is reached only
// after HasGlobal(ViewAllAudits) returned false, so HasIn's global
// short-circuit cannot fire and this reduces to the per-team grant map.
//
// Deliberately NOT a bare len(set.TeamIDs()) > 0check: TeamIDs() is any
// team-scoped grant on ANY role, so gating on it would mean granting, say,
// grc-platform-audit-internal-team at AUDIT_TEAM 2 silently promotes that
// holder from ScopeOwned to ScopeTeam — a visibility change from a pure data
// operation, with no code review. Gating on the privilege instead keeps the
// authority with AUDIT_VIEW_ALL_AUDITS, which only management carries, so a
// team-scoped grant on any other role stays inert by construction.
func managedTeamIDs(set *grant.Set) []int {
	out := []int{}
	for _, id := range set.TeamIDs() {
		if set.HasIn(privilege.ViewAllAudits, id) {
			out = append(out, id)
		}
	}
	return out
}

// controlInScope enforces the same row-level visibility as getControl
// (handler/control.go) for a single control. Any endpoint that reads data
// scoped to one control — comments, trail history, evidence, population —
// must call this (or getControl's inline equivalent) before serving data,
// since ViewAudits/AddComment etc. are coarse booleans with no row scope of
// their own. Writes 404, never 403, so scope can't be probed to distinguish
// "doesn't exist" from "exists outside your scope."
func controlInScope(w http.ResponseWriter, r *http.Request, controlSvc auditservice.ControlService, auditID, controlID int) bool {
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
	inScope, err := controlSvc.InScope(ctx, auditID, controlID, scope, userID, managedTeamIDs(auth.Grants(ctx)))
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

// auditInScope is controlInScope's audit-level counterpart, mirroring getAudit
// (handler/audit.go) for endpoints that read across a whole audit rather than
// one control (e.g. the audit-wide activity log).
func auditInScope(w http.ResponseWriter, r *http.Request, auditSvc auditservice.AuditService, auditID int) bool {
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
	inScope, err := auditSvc.InScope(ctx, auditID, scope, userID, managedTeamIDs(auth.Grants(ctx)))
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

// frameworkInScope gates listFrameworkControls: a caller must not read an
// arbitrary framework's control catalog outside their audit scope.
func frameworkInScope(w http.ResponseWriter, r *http.Request, frameworkSvc auditservice.FrameworkService, frameworkID int) bool {
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
	inScope, err := frameworkSvc.FrameworkInScope(ctx, frameworkID, scope, userID, managedTeamIDs(auth.Grants(ctx)))
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

// deriveWorkQueueClass computes which control-lifecycle bucket is the actor's
// action queue, from privileges. Reviewers (compliance/admin) review; submitters
// without review (internal team) submit; auditors validate; everyone else — most
// notably read-only management, which holds ViewAllAudits but no action privilege
// — has no action queue.
func deriveWorkQueueClass(ctx context.Context) model.WorkQueueClass {
	switch {
	case auth.HasPrivilege(ctx, privilege.ReviewEvidence):
		return model.WorkQueueClassReview
	case auth.HasPrivilege(ctx, privilege.SubmitEvidence):
		return model.WorkQueueClassSubmission
	case auth.HasPrivilege(ctx, privilege.ValidateEvidence):
		return model.WorkQueueClassValidation
	default:
		return model.WorkQueueClassNone
	}
}

func (h *dashboardHandler) getDashboard(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	user := auth.FromContext(r.Context())

	f := model.DashboardFilter{}
	if user != nil {
		f.UserID = user.UserID
	}
	f.ViewScope, f.WorkQueueScope = deriveScopes(r.Context())
	f.ScopeTeamIDs = managedTeamIDs(auth.Grants(r.Context()))
	f.WorkQueueClass = deriveWorkQueueClass(r.Context())

	data, err := h.svc.Get(r.Context(), f)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Failed to load dashboard data.")
		return
	}

	response.WriteJSONValue(w, http.StatusOK, data)
}

func (h *dashboardHandler) getWorkQueue(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ViewAudits) {
		return
	}
	user := auth.FromContext(r.Context())

	f := model.DashboardFilter{}
	if user != nil {
		f.UserID = user.UserID
	}
	f.ViewScope, f.WorkQueueScope = deriveScopes(r.Context())
	f.ScopeTeamIDs = managedTeamIDs(auth.Grants(r.Context()))
	f.WorkQueueClass = deriveWorkQueueClass(r.Context())

	q := r.URL.Query()
	tab := model.WorkQueueTab(q.Get("tab"))
	if tab == "" {
		tab = model.WorkQueueTabActionItems
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	for _, v := range q["teamIds"] {
		if id, err := strconv.Atoi(v); err == nil {
			f.TeamIDs = append(f.TeamIDs, id)
		}
	}
	for _, v := range q["ownerIds"] {
		if id, err := strconv.Atoi(v); err == nil {
			f.OwnerIDs = append(f.OwnerIDs, id)
		}
	}
	for _, v := range q["auditIds"] {
		if id, err := strconv.Atoi(v); err == nil {
			f.AuditIDs = append(f.AuditIDs, id)
		}
	}
	for _, v := range q["statuses"] {
		if v != "" {
			f.Statuses = append(f.Statuses, v)
		}
	}
	f.ControlNumber = strings.TrimSpace(q.Get("controlNumber"))
	f.DueSortDesc = q.Get("dueSort") == "desc"

	p, err := h.svc.GetWorkQueuePage(r.Context(), f, tab, page, limit)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	response.WriteJSONValue(w, http.StatusOK, p)
}

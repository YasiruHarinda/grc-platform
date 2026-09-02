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
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// requireCallerUUID extracts the caller's Asgardeo id and writes a 401 when
// the request carries no authenticated user. Returns ("", false) on failure.
//
// Was requireUserEmail, returning email with a subject fallback — the
// platform is removing email as an identity, so subject is now the only
// answer, not a fallback for a missing claim. This is what created_by/
// updated_by are stamped with, what notifyRiskEvent's actor resolves through,
// and what every ownership check (action-plan owner, evidence uploader,
// high-risk escalation commenter) compares the caller against — see
// internal/user's uuid field and internal/directory for how a uuid becomes a
// displayable name again.
func requireCallerUUID(w http.ResponseWriter, r *http.Request) (string, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return "", false
	}
	return user.Subject, true
}

// parseRiskID extracts and validates the {id} path parameter.
func parseRiskID(w http.ResponseWriter, r *http.Request) (int, bool) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		response.WriteError(w, http.StatusBadRequest, "invalid risk id")
		return 0, false
	}
	return id, true
}

// splitCSV splits a comma-separated query param into trimmed, non-empty parts.
func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// splitCSVInts is splitCSV for comma-separated integer IDs; non-numeric or
// non-positive entries are silently dropped rather than erroring the request.
func splitCSVInts(raw string) []int {
	var out []int
	for _, s := range splitCSV(raw) {
		if id, err := strconv.Atoi(s); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// A caller's standing is now recorded, not inferred.
//
// Three hand-maintained privilege allowlists used to live here — seesEveryRisk,
// isTeamScopedOnly and isActionOwnerOnly — each reverse-engineering "what is
// this person" from the shape of their privilege set, because nothing recorded
// it. Each carried a comment warning that it had to be kept in sync by hand,
// and each failed in the dangerous direction when it wasn't: a new privilege
// omitted from seesEveryRisk left its holder wrongly team-scoped, and one
// omitted from isActionOwnerOnly's list over-scoped an action owner into seeing
// a whole register.
//
// user_role_grant answers all three questions directly:
//
//	seesEveryRisk      → holds RISK_VIEW_RISKS at GLOBAL scope
//	isTeamScopedOnly   → doesn't, but holds some grant
//	isActionOwnerOnly  → holds no grants at all
//
// Nothing needs updating when a privilege is added: none of these consult a
// list of privileges, and the one that names a privilege names the single one
// that actually confers the thing being decided.

// callerGrants returns the caller's resolved grant set. Never nil in
// production; nil only in local dev, where every Set method reports "holds
// nothing" and auth.HasPrivilege* short-circuit to allow-all instead.
func callerGrants(ctx context.Context) *grant.Set {
	return auth.Grants(ctx)
}

// seesEveryRisk reports whether the caller views every register unfiltered.
//
// Asks whether they hold RISK_VIEW_RISKS *at GLOBAL scope*, not merely whether
// some global grant exists. A platform admin holding only MANAGE_USERS globally
// must not be treated as unrestricted: paired with any narrow risk grant that
// gets them past the route gate, that would hand them every risk in the
// system.
func seesEveryRisk(ctx context.Context) bool {
	// Local dev allows every privilege check, so it must also see every risk.
	// These helpers read the grant set directly instead of going through
	// auth.HasPrivilege, so they have to apply that mode themselves.
	return auth.AllowAll(ctx) || callerGrants(ctx).HasGlobal(privilege.ViewRisks)
}

// isTeamScopedOnly reports whether the caller is limited to the teams they hold
// a grant on — the usual case for a Risk Assigner or Risk Owner.
func isTeamScopedOnly(ctx context.Context) bool {
	return !seesEveryRisk(ctx) && !callerGrants(ctx).IsEmpty()
}

// holdsNoGrants reports whether the caller holds no role anywhere.
//
// Not the same as unauthorised: they reach exactly the risks they are
// personally named on. An Action Owner may be any employee — the picker
// deliberately accepts people who have never held a platform role — so this is
// an ordinary state, not a broken one.
func holdsNoGrants(ctx context.Context) bool {
	// Never true in local dev: there is no grant set there, and treating that
	// as "holds nothing" would scope a developer to the risks they happen to
	// own an action plan on.
	return !auth.AllowAll(ctx) && callerGrants(ctx).IsEmpty()
}

// callerUserID resolves the authenticated caller to their internal user id,
// the same uuid lookup handleListRisks' Action Owner list scoping uses.
// Returns (nil, nil) — not an error — when the caller has no platform user
// row, so callers can fail closed on that case themselves.
func (d *Deps) callerUserID(ctx context.Context) (*int, error) {
	userInfo := auth.FromContext(ctx)
	if userInfo == nil {
		return nil, nil
	}
	caller, err := d.Users.GetByUUID(ctx, userInfo.Subject)
	if err != nil {
		return nil, err
	}
	if caller == nil {
		return nil, nil
	}
	return &caller.ID, nil
}

// canOverrideAssigneeIn reports whether the caller may act in place of whoever
// a risk names for a step — the compliance-admin escape hatch that keeps a risk
// from deadlocking when its named owner/approver has left or is unavailable.
//
// registerID must be the risk's SOURCE register. This is the highest-consequence
// check in the module: it bypasses every per-risk identity gate. On the unscoped
// union it would let a compliance approver scoped to one register override
// identity gates on every risk in every register — precisely the cross-scope
// leak this migration exists to prevent.
//
// RISK_COMPLIANCE_APPROVE stands in for "is a compliance admin": among the
// seeded roles only grc-platform-risk-compliance-admin holds it. Testing the
// privilege rather than the role name keeps this consistent with the rest of
// the module, which never checks a role name.
func canOverrideAssigneeIn(ctx context.Context, registerID int) bool {
	return auth.HasPrivilegeIn(ctx, privilege.ComplianceApproveRisk, registerID)
}

// sourceRegisterOf returns the risk's source register — the scope every
// authority check on that risk is relative to. Handlers that hold only a risk
// id use it to ask a scoped question instead of an unscoped one.
func (d *Deps) sourceRegisterOf(ctx context.Context, riskID int) (int, error) {
	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		return 0, err
	}
	return detail.SourceRegisterID, nil
}

// requireRiskActor enforces a per-risk identity gate. Holding the right
// privilege only answers "may this user perform this action on *some* risk"; it
// does not answer "are they the person *this* risk named for it". Both must
// hold, so callers run this after their auth.RequirePrivilege check.
//
// registerID is the risk's source register, used for the compliance-admin
// override. Returns true when the caller is wantUserID or can override there.
// Otherwise it writes the response and returns false. actor names the role in
// the error message, e.g. "Risk Owner" or "Risk Assigner".
func (d *Deps) requireRiskActor(w http.ResponseWriter, r *http.Request, wantUserID, registerID int, actor string) bool {
	if canOverrideAssigneeIn(r.Context(), registerID) {
		return true
	}
	callerID, err := d.callerUserID(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return false
	}
	// A caller with no platform user row can never match, so they always 403 —
	// same outcome as a mismatch, deliberately not distinguished in the message.
	if callerID == nil || *callerID != wantUserID {
		response.WriteError(w, http.StatusForbidden,
			fmt.Sprintf("only this risk's %s may perform this action", actor))
		return false
	}
	return true
}

// requireRiskAssigner is requireRiskActor for the assigner-side actions (edit,
// mark complete, resubmit, cancel), which unlike the approval handlers don't
// otherwise need the risk detail. It loads the risk itself so each call site
// stays a single line.
// requireRiskAssigner enforces BOTH halves of an assigner-side action (edit,
// mark complete, resubmit, cancel) against one loaded risk:
//
//  1. the caller holds priv **in that risk's source register**, and
//  2. they are the risk's assigner (or can override there).
//
// The privilege check lives in here rather than at each call site on purpose.
// Handlers used to run an unscoped auth.RequirePrivilege first, which answers
// "could this user edit *some* risk" — true for anyone holding the privilege in
// any register. Taking the privilege as a parameter makes the scoped check the
// only way to call this, so a handler cannot accidentally keep the weaker one.
//
// The risk is always loaded now: both the scope and the override depend on the
// source register, which cannot be known without it. The old short-circuit
// saved a lookup by asking an unscoped question, and the unscoped question is
// precisely the bug.
func (d *Deps) requireRiskAssigner(w http.ResponseWriter, r *http.Request, riskID int, priv string) bool {
	detail, err := d.Risk.GetByID(r.Context(), riskID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return false
	}
	if !auth.RequirePrivilegeIn(r.Context(), w, priv, detail.SourceRegisterID) {
		return false
	}
	return d.requireRiskActor(w, r, detail.AssignerID, detail.SourceRegisterID, "Risk Assigner")
}

// riskVisibleToCaller reports whether the caller may view riskID's data — the
// by-id counterpart to handleListRisks' list scoping, closing the gap where
// that scoping would otherwise be cosmetic (a caller restricted in the list
// could still read any risk directly by id).
//
// It implements the read half of the access rule:
//
//	See a risk if you hold a GLOBAL grant, OR ANY grant on a team that is
//	either its source register or its assignment team, OR you are personally
//	named on it.
//
// Reading is team-membership based, not scope-basis based: belonging to a
// team at all — through any role, however that role scopes its authority —
// is enough to see risks raised there or routed there. Acting is the narrow
// half: RequirePrivilegeIn(..., SourceRegisterID) still requires the specific
// privilege in the risk's SOURCE register, so a Risk Owner whose team merely
// raised this risk can see it but still cannot approve it.
func (d *Deps) riskVisibleToCaller(ctx context.Context, riskID int) (bool, error) {
	if seesEveryRisk(ctx) {
		return true, nil
	}

	risk, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		return false, err
	}

	for _, id := range callerGrants(ctx).AllScopeIDs() {
		if id == risk.SourceRegisterID || id == risk.AssignmentTeamID {
			return true, nil
		}
	}

	// Identity axis: named on this risk, which needs no grant at all. This is
	// what lets an Action Owner — who may be any employee, holding no role
	// anywhere — reach the one risk they were handed.
	callerID, err := d.callerUserID(ctx)
	if err != nil || callerID == nil {
		return false, err
	}
	if risk.OwnerID == *callerID ||
		risk.AssignerID == *callerID ||
		risk.ManagementApproverID == *callerID {
		return true, nil
	}
	plans, err := d.ActionPlan.List(ctx, riskID)
	if err != nil {
		return false, err
	}
	for _, p := range plans {
		if p.ActionOwnerID != nil && *p.ActionOwnerID == *callerID {
			return true, nil
		}
	}
	return false, nil
}

// handleListRisks serves GET /api/v1/risks.
// Query params:
//   - statuses:        comma-separated workflow status values
//   - team_id:          comma-separated source register IDs
//   - level:            comma-separated LOW | MEDIUM | HIGH values
//   - search:           matched against risk_code and risk_title
//   - risk_type:        comma-separated NEW | UPDATED values
//   - owner_id:          comma-separated owner user IDs
//   - submitted_from/to: created_at date range (YYYY-MM-DD, inclusive)
//   - due_from/to:       implementation_date range (YYYY-MM-DD, inclusive)
//   - due_overdue:       "true" to additionally restrict to implementation_date < today
func (d *Deps) handleListRisks(w http.ResponseWriter, r *http.Request) {
	// No privilege gate: visibility here is decided entirely by the scoping
	// below — seesEveryRisk / isTeamScopedOnly / holdsNoGrants — which together
	// implement the §3 access rule (GLOBAL grant, OR any grant on a team the
	// risk touches, OR named on it via the identity axis). A RequirePrivilege
	// (ViewRisks) gate here would 403 an Action Owner who holds no role at all
	// before the holdsNoGrants branch could scope them to the risks they are
	// named on — the same mistake handleUpdateActionPlanStep documents. Each
	// branch fails closed: an unresolvable or grant-less-and-unnamed caller
	// gets an empty page, never an unscoped one.
	if _, ok := requireCallerUUID(w, r); !ok {
		return
	}
	q := r.URL.Query()

	var filter model.ListRisksFilter
	filter.Statuses = splitCSV(q.Get("statuses"))
	filter.TeamIDs = splitCSVInts(q.Get("team_id"))
	filter.Levels = splitCSV(q.Get("level"))
	filter.Search = q.Get("search")
	filter.RiskTypes = splitCSV(q.Get("risk_type"))
	filter.OwnerIDs = splitCSVInts(q.Get("owner_id"))
	filter.SubmittedFrom = q.Get("submitted_from")
	filter.SubmittedTo = q.Get("submitted_to")
	filter.DueFrom = q.Get("due_from")
	filter.DueTo = q.Get("due_to")
	filter.DueOverdueOnly = q.Get("due_overdue") == "true"
	filter.OpenEscalationOnly = q.Get("open_escalation") == "true"
	// Taken from the authenticated caller, never the query string: this widens
	// visibility, so a client-supplied value would let anyone read any risk
	// they could name a lead for.
	if user := auth.FromContext(r.Context()); user != nil {
		filter.EscalationLeadUUID = user.Subject
	}

	filter.Limit = 50
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
			filter.Limit = v
		}
	}
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			filter.Offset = v
		}
	}

	// Zero-grant scoping: a caller holding no role anywhere sees only the risks
	// they are personally named on. An Action Owner may be any employee, with
	// no grants at all, so this is an ordinary state rather than a broken one.
	//
	// NARROWER THAN THE BY-ID RULE, DELIBERATELY. riskVisibleToCaller admits
	// anyone named on a risk in any capacity — owner, assigner, management
	// approver, or action owner — but the list filter can only express
	// action_owner_id today. The difference fails closed: such a caller may see
	// fewer rows in the list than they can open directly, never more. Widening
	// it needs a "named in any capacity" filter on the entity's risk search.
	//
	// Fails closed the other way too: leaving the filter unset when the caller
	// cannot be resolved would hand them the entire register, so an
	// unresolvable caller gets an empty page, never an unscoped one.
	if holdsNoGrants(r.Context()) {
		callerUUID, ok := requireCallerUUID(w, r)
		if !ok {
			return
		}
		caller, err := d.Users.GetByUUID(r.Context(), callerUUID)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		if caller == nil {
			// Authenticated but with no platform user row: they cannot be any
			// plan's action_owner_id, so an empty page is the truthful scoped
			// result rather than an error.
			response.WriteJSONValue(w, http.StatusOK, model.RiskListPage{
				Items:  []*model.RiskListItem{},
				Total:  0,
				Offset: filter.Offset,
				Limit:  filter.Limit,
			})
			return
		}
		filter.ActionOwnerID = &caller.ID
	}

	// Team scoping: a caller with grants but no GLOBAL one sees every risk
	// belonging to a team they hold ANY grant on — raised there or routed
	// there, regardless of that grant's own scope_basis. Membership is what
	// decides visibility here; authority over those risks still follows the
	// source register alone, enforced separately by RequirePrivilegeIn.
	if isTeamScopedOnly(r.Context()) {
		teamIDs := callerGrants(r.Context()).AllScopeIDs()
		if len(teamIDs) == 0 {
			// Reachable: a caller may hold only GLOBAL grants for a privilege
			// other than ViewRisks (see seesEveryRisk's caution), so they carry
			// no RISK_TEAM-scoped grant at all. An empty list means
			// "unrestricted" downstream, so this must fail closed rather than
			// hand them every risk.
			response.WriteJSONValue(w, http.StatusOK, model.RiskListPage{
				Items:  []*model.RiskListItem{},
				Total:  0,
				Offset: filter.Offset,
				Limit:  filter.Limit,
			})
			return
		}
		filter.ScopeSourceRegisterIDs = teamIDs
		filter.ScopeAssignmentTeamIDs = teamIDs
	}

	page, err := d.Risk.List(r.Context(), filter)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.enrichListItems(r.Context(), page.Items)
	response.WriteJSONValue(w, http.StatusOK, page)
}

// handleGetRisk serves GET /api/v1/risks/{id}.
func (d *Deps) handleGetRisk(w http.ResponseWriter, r *http.Request) {
	// No privilege gate: riskVisibleToCaller below is the whole check. It
	// implements the §3 read rule (GLOBAL grant, OR any grant on the risk's
	// source register or assignment team, OR named on it), and 404s anyone it
	// doesn't admit. A RequirePrivilege(ViewRisks) gate here would 403 an
	// Action Owner holding no role before that identity check could let them
	// reach the one risk they were handed.
	if _, ok := requireCallerUUID(w, r); !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	visible, err := d.riskVisibleToCaller(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !visible {
		response.WriteError(w, http.StatusNotFound, response.ErrMsgNotFound)
		return
	}

	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	detail.EffectivePrivileges = effectivePrivilegesFor(r.Context(), detail.SourceRegisterID, detail.AssignmentTeamID)
	d.enrichDetail(r.Context(), detail)
	response.WriteJSONValue(w, http.StatusOK, detail)
}

// effectivePrivilegesFor resolves what the caller may do on a risk, for the
// UI to render its action buttons from.
//
// Takes both of a risk's team dimensions, not just its source register:
// RISK_OWNER_APPROVE/RISK_OWNER_REJECT/RISK_ASSESS are held by
// grc-platform-risk-owner, scoped ASSIGNMENT_TEAM, so a caller whose grant
// sits on the risk's assignment team rather than its source register would
// otherwise never see the button for an action they are fully entitled to —
// see auth.HasPrivilegeInEither for the full reasoning.
//
// In local dev (no privilege store configured) every check is allowed, so the
// honest answer is "everything the module defines" rather than an empty list —
// otherwise the UI would hide every action in the one mode where the server
// permits them all, and local dev would look broken instead of permissive.
func effectivePrivilegesFor(ctx context.Context, sourceRegisterID, assignmentTeamID int) []string {
	if privilege.FromContext(ctx) == nil {
		return privilege.AllRiskPrivileges()
	}
	return callerGrants(ctx).PrivilegesInEither(sourceRegisterID, assignmentTeamID)
}

// handleUpdateRisk serves PUT /api/v1/risks/{id}.
// Updating any restricted field (implementation_date, email_subject, action_steps)
// on an IN_REMEDIATION risk moves it to PENDING_AMENDMENT.
func (d *Deps) handleUpdateRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if !d.requireRiskAssigner(w, r, id, privilege.UpdateRisk) {
		return
	}

	var req model.UpdateRiskRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	if req.RiskTitle == "" {
		response.WriteError(w, http.StatusBadRequest, "risk_title is required")
		return
	}
	if req.RiskDescription == "" {
		response.WriteError(w, http.StatusBadRequest, "risk_description is required")
		return
	}
	if req.EmailSubject == "" {
		response.WriteError(w, http.StatusBadRequest, "email_subject is required")
		return
	}

	// IdentifiedByType == "" means "leave Identified By unchanged" — see the
	// COALESCE-on-empty convention this maps onto in the repository. Only
	// validate/resolve when the caller is actually setting it this request.
	if req.IdentifiedByType != "" {
		switch req.IdentifiedByType {
		case model.IdentifiedByEmployee:
			if req.IdentifiedByEmail == nil || strings.TrimSpace(*req.IdentifiedByEmail) == "" {
				response.WriteError(w, http.StatusBadRequest, "identified_by_email is required when identified_by_type is "+model.IdentifiedByEmployee)
				return
			}
			name, err := d.resolveIdentifiedByEmployee(r.Context(), *req.IdentifiedByEmail)
			if err != nil {
				response.MapServiceError(r.Context(), w, err, "Unable to verify the identifying employee. Please try again.")
				return
			}
			req.IdentifiedByName = &name
		case model.IdentifiedByExternalPerson, model.IdentifiedByTool:
			if req.IdentifiedByName == nil || strings.TrimSpace(*req.IdentifiedByName) == "" {
				response.WriteError(w, http.StatusBadRequest, "identified_by_name is required when identified_by_type is "+req.IdentifiedByType)
				return
			}
			trimmed := strings.TrimSpace(*req.IdentifiedByName)
			req.IdentifiedByName = &trimmed
		default:
			response.WriteError(w, http.StatusBadRequest, "identified_by_type must be "+model.IdentifiedByEmployee+", "+model.IdentifiedByExternalPerson+", or "+model.IdentifiedByTool)
			return
		}
	}

	if err := d.Risk.Update(r.Context(), id, req, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleOwnerApproveRisk serves POST /api/v1/risks/{id}/owner-approve.
// Handles PENDING_RISK_OWNER_APPROVAL, PENDING_AMENDMENT, and PENDING_OWNER_COMPLETION_APPROVAL.
//
// Gated on RISK_OWNER_APPROVE *and* on being this risk's own owner_id: belonging
// to the Risk Owner role, or to the risk's team, does not make someone the owner
// of every risk in it. Compliance admins bypass — see requireRiskActor.
func (d *Deps) handleOwnerApproveRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilegeInEither(r.Context(), w, privilege.OwnerApproveRisk, detail.SourceRegisterID, detail.AssignmentTeamID) {
		return
	}
	if !d.requireRiskActor(w, r, detail.OwnerID, detail.SourceRegisterID, "Risk Owner") {
		return
	}
	// Captured before the transition: OwnerApprove decides where the risk goes
	// from the status it had on entry, and re-reading afterwards would race
	// with a concurrent action.
	fromStatus := detail.WorkflowStatus
	if err := d.Risk.OwnerApprove(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordOwnerApprove(r.Context(), id, detail, fromStatus, by)
	d.notifyAfterOwnerApprove(id, detail, fromStatus, by)
	w.WriteHeader(http.StatusNoContent)
}

// notifyAfterOwnerApprove mirrors OwnerApprove's routing to notify whoever the
// risk just landed on. It recomputes the destination from the same inputs the
// service used rather than re-reading the risk, so the two must be kept in step
// — see riskservice.OwnerApprove.
func (d *Deps) notifyAfterOwnerApprove(id int, detail *model.RiskDetail, fromStatus, by string) {
	acceptHigh := detail.TreatmentStrategy != nil && *detail.TreatmentStrategy == "ACCEPT" &&
		detail.GrossScore != nil && detail.GrossScore.RiskLevel == "HIGH"

	switch fromStatus {
	case model.StatusPendingOwnerApproval, model.StatusPendingAmendment:
		if acceptHigh {
			// → PENDING_MANAGEMENT_APPROVAL
			d.notifyRiskEvent(emailer.EventPendingMgmtApproval, id, []int{detail.ManagementApproverID}, by, "")
			return
		}
		// → PENDING_COMPLIANCE_REVIEW: a role, not a named individual. A
		// dedicated event, not EventPendingMgmtApproval — that template
		// hardcodes the Management Approver as who needs to act, which is
		// wrong for compliance admin recipients.
		d.notifyComplianceAdmins(emailer.EventPendingComplianceReview, id, detail.SourceRegisterID, by, "")

	case model.StatusPendingOwnerCompletion:
		if acceptHigh {
			// → PENDING_MANAGEMENT_CLOSURE_APPROVAL
			d.notifyRiskEvent(emailer.EventPendingMgmtClosure, id, []int{detail.ManagementApproverID}, by, "")
			return
		}
		// → PENDING_COMPLIANCE_CLOSURE: a role, not a named individual. See
		// the PENDING_COMPLIANCE_REVIEW branch above for why this is its own
		// event rather than EventPendingMgmtClosure.
		d.notifyComplianceAdmins(emailer.EventPendingComplianceClosure, id, detail.SourceRegisterID, by, "")
	}
}

// handleManagementApproveRisk serves POST /api/v1/risks/{id}/management-approve.
// Serves both management stages — see riskservice.ManagementApprove.
func (d *Deps) handleManagementApproveRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	callerID, err := d.callerUserID(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// The risk is loaded for its source register, which both the scoped
	// privilege check and the override are relative to.
	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilegeIn(r.Context(), w, privilege.ManagementApproveRisk, detail.SourceRegisterID) {
		return
	}
	if err := d.Risk.ManagementApprove(r.Context(), id, by, callerID,
		canOverrideAssigneeIn(r.Context(), detail.SourceRegisterID)); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), id, by, model.HistoryApprove, model.HistoryDetails{Role: "Management"})
	// Both management stages hand off to a compliance stage, whose recipient
	// is a role rather than a named individual — which event depends on
	// which stage this was, captured before the transition above. Not
	// EventComplianceApproved: that event means compliance has ALREADY
	// approved and remediation may begin, the opposite of what just
	// happened here.
	complianceEvent := emailer.EventPendingComplianceReview
	if detail.WorkflowStatus == model.StatusPendingManagementClosure {
		complianceEvent = emailer.EventPendingComplianceClosure
	}
	d.notifyComplianceAdmins(complianceEvent, id, detail.SourceRegisterID, by, "")
	w.WriteHeader(http.StatusNoContent)
}

// handleApproveRisk serves POST /api/v1/risks/{id}/approve.
// Compliance approval: PENDING_COMPLIANCE_REVIEW → IN_REMEDIATION.
func (d *Deps) handleApproveRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	// Compliance approval is deliberately not identity-gated (it is a
	// role-wide action), so the scoped privilege check is the ONLY thing
	// standing between a compliance approver in one register and a risk in
	// another. It cannot be left unscoped.
	registerID, err := d.sourceRegisterOf(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilegeIn(r.Context(), w, privilege.ComplianceApproveRisk, registerID) {
		return
	}
	if err := d.Risk.Approve(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), id, by, model.HistoryApprove, model.HistoryDetails{
		Role: "Compliance", From: model.StatusPendingComplianceReview, To: model.StatusInRemediation,
	})
	// Remediation can now start, so the two people who have to do it are told:
	// the assigner who owns the risk's progress and the action plan's owner.
	d.notifyRiskEvent(emailer.EventComplianceApproved, id, d.remediationRecipients(r.Context(), id), by, "")
	w.WriteHeader(http.StatusNoContent)
}

// remediationRecipients returns the risk's assigner plus every action-plan
// owner. Action owners come from the plan list rather than detail.action_plan,
// which only ever embeds the STANDARD plan; a risk may have several. Failures
// are logged and degrade to "assigner only" rather than losing the whole
// notification.
func (d *Deps) remediationRecipients(ctx context.Context, riskID int) []int {
	ids := []int{}
	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("risk notification: failed to load risk for recipients", "riskId", riskID, "err", err)
		return ids
	}
	ids = append(ids, detail.AssignerID)

	plans, err := d.ActionPlan.List(ctx, riskID)
	if err != nil {
		slog.Warn("risk notification: failed to list action plans for recipients", "riskId", riskID, "err", err)
		return ids
	}
	for _, p := range plans {
		if p.ActionOwnerID != nil {
			ids = append(ids, *p.ActionOwnerID)
		}
	}
	return ids // notifyRiskEvent de-duplicates
}

// rejectPrivilegeFor maps a workflow status to the privilege required to reject
// at that stage. Defaults to OwnerRejectRisk for all owner-stage states.
func rejectPrivilegeFor(status string) string {
	switch status {
	case model.StatusPendingManagementApproval, model.StatusPendingManagementClosure:
		return privilege.ManagementRejectRisk
	case model.StatusPendingComplianceReview:
		return privilege.ComplianceRejectRisk
	default: // PENDING_RISK_OWNER_APPROVAL, PENDING_AMENDMENT, PENDING_OWNER_COMPLETION_APPROVAL
		return privilege.OwnerRejectRisk
	}
}

// handleRejectRisk serves POST /api/v1/risks/{id}/reject.
// Routes to PENDING_REVISION from any pending-approval stage; stores rejection_stage.
// The required privilege depends on which stage the risk is currently at.
func (d *Deps) handleRejectRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}

	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// Either dimension, not just the source register: at the owner stage this
	// is RISK_OWNER_REJECT, which grc-platform-risk-owner holds scoped
	// ASSIGNMENT_TEAM (see auth.HasPrivilegeInEither). Management/compliance
	// stages only ever grant their privilege SOURCE_REGISTER-scoped, so the
	// assignment-team side of this check simply never matches for them.
	if !auth.RequirePrivilegeInEither(r.Context(), w, rejectPrivilegeFor(detail.WorkflowStatus), detail.SourceRegisterID, detail.AssignmentTeamID) {
		return
	}
	// Rejecting is restricted to the same named individual who would have
	// approved at this stage — the identity gate mirrors the approve path
	// exactly, so a rejection can't come from someone who couldn't have
	// approved. Compliance stages have no named individual and stay
	// privilege-only.
	switch detail.WorkflowStatus {
	case model.StatusPendingManagementApproval, model.StatusPendingManagementClosure:
		if !d.requireRiskActor(w, r, detail.ManagementApproverID, detail.SourceRegisterID, "Management Approver") {
			return
		}
	case model.StatusPendingOwnerApproval, model.StatusPendingAmendment, model.StatusPendingOwnerCompletion:
		if !d.requireRiskActor(w, r, detail.OwnerID, detail.SourceRegisterID, "Risk Owner") {
			return
		}
	}

	var req model.RejectRiskRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	if err := d.Risk.Reject(r.Context(), id, req, detail.WorkflowStatus, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), id, by, model.HistoryReject, model.HistoryDetails{
		From: detail.WorkflowStatus, To: model.StatusPendingRevision,
		Stage: rejectionStageFor(detail.WorkflowStatus), Comment: req.RejectionComment,
	})
	// One notification covers every rejection stage: whoever rejected it, the
	// risk lands back with the assigner, who is the one who has to act.
	d.notifyRiskEvent(emailer.EventRejected, id, []int{detail.AssignerID}, by, req.RejectionComment)
	w.WriteHeader(http.StatusNoContent)
}

// handleCompleteRisk serves POST /api/v1/risks/{id}/complete.
// Transitions IN_REMEDIATION → PENDING_OWNER_COMPLETION_APPROVAL.
func (d *Deps) handleCompleteRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if !d.requireRiskAssigner(w, r, id, privilege.CompleteRisk) {
		return
	}
	if err := d.Risk.Complete(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), id, by, model.HistoryComplete, model.HistoryDetails{
		From: model.StatusInRemediation, To: model.StatusPendingOwnerCompletion,
	})
	// Submitting for completion approval is what finally closes an escalation:
	// up to this point the risk stayed in the Overdue tab even while back
	// IN_REMEDIATION. Best-effort — the risk has already moved on, and failing
	// the request here would be worse than leaving it in the Overdue tab.
	if err := d.Escalation.ResolveOpen(r.Context(), id, by); err != nil {
		slog.Warn("failed to resolve open escalations on completion", "riskId", id, "err", err)
	}
	d.notifyOwnerOfCompletion(r.Context(), id, by)
	w.WriteHeader(http.StatusNoContent)
}

// handleResubmitRisk serves POST /api/v1/risks/{id}/resubmit.
// Transitions PENDING_REVISION → PENDING_RISK_OWNER_APPROVAL and clears rejection info.
func (d *Deps) handleResubmitRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if !d.requireRiskAssigner(w, r, id, privilege.SubmitRisk) {
		return
	}
	// Resubmit routes on the rejection stage, so who to notify depends on where
	// it goes back to — mirroring riskservice.Resubmit.
	detail, err := d.Risk.GetByID(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	stage := ""
	if detail.RejectionStage != nil {
		stage = *detail.RejectionStage
	}
	if err := d.Risk.Resubmit(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), id, by, model.HistorySubmit, model.HistoryDetails{
		From: model.StatusPendingRevision, To: resubmitTargetFor(stage), Stage: stage,
	})
	switch stage {
	case "COMPLETION_OWNER":
		d.notifyRiskEvent(emailer.EventPendingOwnerClosure, id, []int{detail.OwnerID}, by, "")
	case "COMPLETION_MANAGEMENT":
		d.notifyRiskEvent(emailer.EventPendingMgmtClosure, id, []int{detail.ManagementApproverID}, by, "")
	default:
		// Back to the start of the chain, which is the risk owner again.
		d.notifyRiskEvent(emailer.EventCreated, id, []int{detail.OwnerID}, by, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCancelRisk serves POST /api/v1/risks/{id}/cancel.
// Soft-deletes a risk by moving it to CANCELLED. Only valid from PENDING_RISK_OWNER_APPROVAL.
func (d *Deps) handleCancelRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	if !d.requireRiskAssigner(w, r, id, privilege.CancelRisk) {
		return
	}
	if err := d.Risk.Cancel(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), id, by, model.HistoryCancel, model.HistoryDetails{To: model.StatusCancelled})
	w.WriteHeader(http.StatusNoContent)
}

// handleCloseRisk serves POST /api/v1/risks/{id}/close.
// Transitions PENDING_COMPLIANCE_CLOSURE → CLOSED.
func (d *Deps) handleCloseRisk(w http.ResponseWriter, r *http.Request) {
	by, ok := requireCallerUUID(w, r)
	if !ok {
		return
	}
	id, ok := parseRiskID(w, r)
	if !ok {
		return
	}
	// No identity gate on closure either — the scoped privilege is the gate.
	registerID, err := d.sourceRegisterOf(r.Context(), id)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	if !auth.RequirePrivilegeIn(r.Context(), w, privilege.CloseRisk, registerID) {
		return
	}
	if err := d.Risk.Close(r.Context(), id, by); err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	d.recordEvent(r.Context(), id, by, model.HistoryClose, model.HistoryDetails{
		From: model.StatusPendingComplianceClosure, To: model.StatusClosed,
	})
	d.notifyRiskClosed(r.Context(), id, registerID, by)
	w.WriteHeader(http.StatusNoContent)
}

// notifyRiskClosed tells everyone with a stake in this risk that it has
// reached CLOSED: the Assigner, Risk Owner, and Management Approver by name,
// plus the Compliance Admin role via notifyComplianceAdmins. Two calls, not
// one, for the same reason NotifyEscalation splits them — a role has no
// single user id to fold in alongside the three named recipients.
func (d *Deps) notifyRiskClosed(ctx context.Context, riskID, registerID int, by string) {
	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("risk notification: failed to load risk for closure", "riskId", riskID, "err", err)
		return
	}
	d.notifyRiskEvent(emailer.EventClosed, riskID,
		[]int{detail.AssignerID, detail.OwnerID, detail.ManagementApproverID}, by, "")
	d.notifyComplianceAdmins(emailer.EventClosed, riskID, registerID, by, "")
}

// notifyOwnerOfCompletion tells the risk's owner that remediation has been
// marked complete and is waiting on their sign-off. Split out so both the
// Complete and Resubmit paths — which land on the same stage — share it.
func (d *Deps) notifyOwnerOfCompletion(ctx context.Context, riskID int, by string) {
	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("risk notification: failed to load risk for owner completion", "riskId", riskID, "err", err)
		return
	}
	d.notifyRiskEvent(emailer.EventPendingOwnerClosure, riskID, []int{detail.OwnerID}, by, "")
}

// rejectionStageFor mirrors riskservice.Reject's stage mapping, so the history
// records the same stage the risk row is stamped with.
func rejectionStageFor(status string) string {
	switch status {
	case model.StatusPendingManagementApproval:
		return "MANAGEMENT"
	case model.StatusPendingComplianceReview:
		return "COMPLIANCE"
	case model.StatusPendingOwnerCompletion:
		return "COMPLETION_OWNER"
	case model.StatusPendingManagementClosure:
		return "COMPLETION_MANAGEMENT"
	default:
		return "OWNER"
	}
}

// resubmitTargetFor mirrors riskservice.Resubmit's routing.
func resubmitTargetFor(rejectionStage string) string {
	switch rejectionStage {
	case "COMPLETION_OWNER":
		return model.StatusPendingOwnerCompletion
	case "COMPLETION_MANAGEMENT":
		return model.StatusPendingManagementClosure
	default:
		return model.StatusPendingOwnerApproval
	}
}

// recordOwnerApprove mirrors riskservice.OwnerApprove's routing so the history
// records where the risk actually went. Kept next to notifyAfterOwnerApprove,
// which recomputes the same thing for the same reason.
func (d *Deps) recordOwnerApprove(ctx context.Context, id int, detail *model.RiskDetail, fromStatus, by string) {
	acceptHigh := detail.TreatmentStrategy != nil && *detail.TreatmentStrategy == "ACCEPT" &&
		detail.GrossScore != nil && detail.GrossScore.RiskLevel == "HIGH"

	to := ""
	switch fromStatus {
	case model.StatusPendingOwnerApproval, model.StatusPendingAmendment:
		to = model.StatusPendingComplianceReview
		if acceptHigh {
			to = model.StatusPendingManagementApproval
		}
	case model.StatusPendingOwnerCompletion:
		to = model.StatusPendingComplianceClosure
		if acceptHigh {
			to = model.StatusPendingManagementClosure
		}
	}
	d.recordEvent(ctx, id, by, model.HistoryApprove, model.HistoryDetails{
		Role: "Risk Owner", From: fromStatus, To: to,
	})
}

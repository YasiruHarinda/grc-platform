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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// handleListManagementApprovers serves GET /api/v1/management-approvers:
// every user who holds RISK_MANAGEMENT_APPROVE, GLOBAL or scoped to one of the
// given teamId query params (repeatable — pass the risk's source register and
// assignment team). A candidate returned here is guaranteed to pass the same
// scoped check handleManagementApproveRisk runs, so picking them can never
// 403 on their first approval.
func (d *Deps) handleListManagementApprovers(w http.ResponseWriter, r *http.Request) {
	d.handleListCandidates(w, r, privilege.ManagementApproveRisk)
}

// handleListRiskOwnerCandidates serves GET /api/v1/risk-owner-candidates:
// every user who holds RISK_OWNER_APPROVE, GLOBAL or scoped to one of the
// given teamId query params. See handleListManagementApprovers — same
// mechanism, different privilege.
func (d *Deps) handleListRiskOwnerCandidates(w http.ResponseWriter, r *http.Request) {
	d.handleListCandidates(w, r, privilege.OwnerApproveRisk)
}

// handleListRiskAssignerCandidates serves GET /api/v1/risk-assigner-candidates:
// every user who holds RISK_CREATE, GLOBAL or scoped to one of the given
// teamId query params (Add Risk passes the chosen source register). Assigning
// a risk to someone who holds no CreateRisk grant in that register is
// meaningless — they could never have raised it there themselves — so this is
// the same "offer exactly who the server would accept" guarantee
// handleListManagementApprovers documents, applied to the assigner field.
func (d *Deps) handleListRiskAssignerCandidates(w http.ResponseWriter, r *http.Request) {
	d.handleListCandidates(w, r, privilege.CreateRisk)
}

// handleListCandidates returns everyone eligible for a role-gated picker
// field. This replaced a live SCIM lookup against an Asgardeo group,
// intersected client-side against the (now deprecated) user_risk_team table —
// two sources unrelated to user_role_grant, the table that actually decides
// whether the picked person can act. A candidate could hold the group but no
// grant in the risk's register, or a grant but not the group, and either way
// they'd 403 the first time they tried to approve. This is now one query
// against the same table the approval check itself reads.
//
// A candidate who doesn't resolve through the identity directory is silently
// dropped from the list, not just shown with a blank name — see resolveCandidates.
//
// Gated on the same privileges as /users/resolve and /employees/search: every
// caller of these pickers is filling in an Add Risk or Edit Risk form, so a
// caller holding neither CreateRisk nor UpdateRisk has no legitimate reason to
// read them — and without this guard any authenticated caller could enumerate
// every internal user holding the picked privilege, with resolved name, email
// and uuid, and sweep teamId to map who holds authority in which register.
// ManageRiskHub is included so a Risk Hub admin editing configuration still
// sees the pickers.
func (d *Deps) handleListCandidates(w http.ResponseWriter, r *http.Request, priv string) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.CreateRisk, privilege.UpdateRisk, privilege.ManageRiskHub) {
		return
	}

	teamIDs := make([]int, 0, len(r.URL.Query()["teamId"]))
	for _, raw := range r.URL.Query()["teamId"] {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			response.WriteError(w, http.StatusBadRequest, "teamId must be a positive integer")
			return
		}
		teamIDs = append(teamIDs, id)
	}

	// Local dev (no privilege store configured): there are no grants to query,
	// and every check elsewhere in this mode allows everything — so every
	// platform user is offered, rather than silently returning zero
	// candidates. keepUnresolved=true carries that same intent through
	// resolveCandidates: local dev without SCIM configured resolves nobody,
	// so the normal drop-unresolved rule would otherwise empty this list too.
	if auth.AllowAll(r.Context()) {
		all, err := d.Users.List(r.Context())
		if err != nil {
			response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
			return
		}
		ids := make([]idUUID, 0, len(all))
		for _, u := range all {
			ids = append(ids, idUUID{id: u.ID, uuid: u.UUID})
		}
		response.WriteJSONValue(w, http.StatusOK, d.resolveCandidates(r.Context(), ids, true))
		return
	}

	candidates, err := d.Grants.Candidates(r.Context(), priv, teamIDs)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	ids := make([]idUUID, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, idUUID{id: c.ID, uuid: c.UUID})
	}
	response.WriteJSONValue(w, http.StatusOK, d.resolveCandidates(r.Context(), ids, false))
}

// idUUID is a candidate's platform id paired with the uuid to resolve their
// name/email through — the two sources handleListCandidates draws candidates
// from (d.Users.List, d.Grants.Candidates) carry this pair in different
// shapes, so callers normalise to this before resolveCandidates.
type idUUID struct {
	id   int
	uuid string
}

// resolveCandidates batch-resolves every candidate's name and email through
// the identity directory. By default it drops anyone who doesn't resolve — a
// candidate with no resolvable identity should not be offered as a pickable
// option at all, matching resolve.go's Action Owner rule: if the directory
// can't name someone, they should not be assignable to a risk in any
// capacity.
//
// keepUnresolved overrides that for handleListCandidates's local-dev path,
// where a nil/unconfigured directory would otherwise resolve nobody and
// silently empty the list that branch explicitly promises to keep full — an
// unresolved candidate there is still returned, with blank Email/DisplayName.
//
// LookupAll's stale-serve behaviour is what keeps the default path resilient:
// a candidate only vanishes from a picker if their uuid has never once
// resolved, not merely because the directory hiccuped on this one request.
func (d *Deps) resolveCandidates(ctx context.Context, candidates []idUUID, keepUnresolved bool) []*userentity.User {
	out := make([]*userentity.User, 0, len(candidates))
	people := map[string]directory.Person{}
	if d.Directory != nil {
		uuids := make([]string, 0, len(candidates))
		for _, c := range candidates {
			uuids = append(uuids, c.uuid)
		}
		people = d.Directory.LookupAll(ctx, uuids)
	}
	for _, c := range candidates {
		p, ok := people[c.uuid]
		if !ok && !keepUnresolved {
			continue
		}
		// RiskTeamIDs is set to an empty (never nil) slice: team membership no
		// longer decides eligibility here — the scoped query upstream already
		// did that — but the response shape is shared with GET /users, whose
		// callers expect this field to always be an array.
		out = append(out, &userentity.User{ID: c.id, UUID: c.uuid, Email: p.Email, DisplayName: p.DisplayName, RiskTeamIDs: []int{}})
	}
	return out
}

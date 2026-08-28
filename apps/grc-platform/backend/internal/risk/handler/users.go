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
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// handleListUsers serves GET /api/v1/risks/users: every active user, for the
// Risk module's general user dropdowns (e.g. the Add Risk form). Risk-only —
// Audit Hub has its own GET /api/v1/audits/users backed by a completely
// separate handler/service/repository stack.
//
// Gated on ViewRisks OR ManageRiskHub — same shape as GET /api/v1/risks/teams
// (team.go): every Risk Hub role that populates the Add Risk pickers this
// feeds is seeded with RISK_VIEW_RISKS, and grc-platform-admin reaches it via
// ManageRiskHub. Without this gate any authenticated caller — an external
// auditor included, since Audit Hub has its own privilege set — could
// enumerate every active platform user's resolved display name and email.
//
// A user who doesn't resolve through the identity directory is dropped from
// the list rather than shown with a blank name — the platform is removing
// stored names/emails entirely, so there is nothing left to fall back to,
// and the same "must resolve to be offered" rule already applies to every
// other picker (see candidates.go's resolveCandidates).
func (d *Deps) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.ViewRisks, privilege.ManageRiskHub) {
		return
	}

	users, err := d.Users.List(r.Context())
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	out := make([]*user.User, 0, len(users))
	switch {
	case d.Directory != nil:
		uuids := make([]string, 0, len(users))
		for _, u := range users {
			uuids = append(uuids, u.UUID)
		}
		people := d.Directory.LookupAll(r.Context(), uuids)
		for _, u := range users {
			p, ok := people[u.UUID]
			if !ok {
				continue
			}
			out = append(out, &user.User{
				ID: u.ID, UUID: u.UUID, Email: p.Email, DisplayName: p.DisplayName,
				Status: u.Status, RiskTeamIDs: u.RiskTeamIDs,
			})
		}
	case auth.AllowAll(r.Context()):
		// Local dev, no SCIM configured: Directory is nil, so nobody could
		// ever resolve, which would silently empty this dropdown in the one
		// mode meant to be permissive. Mirrors resolveCandidates'
		// keepUnresolved path (candidates.go) — return the raw rows, which
		// still carry their own Email/DisplayName directly from the entity for
		// as long as those columns exist (see user.User).
		out = users
	}
	response.WriteJSONValue(w, http.StatusOK, out)
}

type resolveUserRequest struct {
	Email string `json:"email"`
}

// handleResolveUser serves POST /api/v1/risks/users/resolve: it links an HR
// entity employee (identified by email, as returned by
// GET /api/v1/risks/employees/search) to an internal user.id, creating the
// user row on the fly if one doesn't exist yet. Used wherever a form needs to
// assign any employee — not just an existing grc-platform user — to an FK
// field (e.g. a risk's Action Owner).
//
// hr_entity is consulted for the WSO2-employee check, not for a name to
// store: the platform stores no display name anymore (see Upsert's call
// below), so there is nothing left for a client-supplied name to corrupt.
// hr_entity and the identity directory answer different questions — hr_entity
// says "is this a current WSO2 employee", which is the check that gate exists
// for; Asgardeo says "does this person have an account", which can outlive
// their employment. Only the first is a reason to refuse on its own; see the
// uuid resolution below for what makes the second one a reason too.
//
// This writes, so it is gated. The privileges are the risk module's because
// that is the only flow that calls it: an employee is resolved to a user id
// while creating or editing a risk. Either privilege is enough — a user who
// may only edit still has to assign an action owner.
func (d *Deps) handleResolveUser(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireAnyPrivilege(r.Context(), w, privilege.CreateRisk, privilege.UpdateRisk) {
		return
	}

	var req resolveUserRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		response.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	emp, err := d.HREntity.GetEmployeeByEmail(r.Context(), req.Email)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, "Unable to reach the employee directory. Please try again.")
		return
	}
	if emp == nil {
		response.WriteError(w, http.StatusUnprocessableEntity, "email does not match an active WSO2 employee")
		return
	}
	// hr_entity's name is used only as this response's last-resort display
	// value (see below) — never stored — so a directory blip doesn't leave
	// the caller unable to see who they just picked.
	hrName := strings.TrimSpace(strings.TrimSpace(emp.FirstName) + " " + strings.TrimSpace(emp.LastName))
	if hrName == "" {
		response.WriteError(w, http.StatusUnprocessableEntity, "email does not match an active WSO2 employee")
		return
	}

	// Resolve the employee's Asgardeo id, so the row is provisioned with the
	// identity they will actually authenticate as — and, now that nothing
	// stores a name, so the identity directory can name them anywhere they
	// get displayed afterward (dropdowns, notifications).
	//
	// Both a confirmed "no such person" and an unreachable directory now
	// refuse the assignment — this changed from a soft degrade (provision
	// without a uuid, backfill later) once user.uuid became NOT NULL: there
	// is no longer a row to create without one. Blocking a legitimate
	// hire's assignment because SCIM timed out for a few seconds is a real
	// cost, but the alternative — Upsert rejecting an empty uuid outright —
	// makes the soft-degrade path impossible to keep; failing closed here,
	// with a message the caller can retry against, is the honest version of
	// that same tradeoff now.
	dirUser, dirErr := d.SCIM.LookupByEmail(r.Context(), req.Email)
	if dirErr != nil {
		slog.WarnContext(r.Context(), "resolve user: identity directory unreachable, refusing to provision without a uuid",
			"err", dirErr)
		response.WriteError(w, http.StatusServiceUnavailable,
			"could not verify this person's identity right now — please try again")
		return
	}
	if dirUser == nil {
		response.WriteError(w, http.StatusUnprocessableEntity,
			"email does not have an Asgardeo account and cannot be assigned")
		return
	}

	// Attribution for the provisioned row: the Compliance Entity records
	// this as created_by/updated_by (see auth.FromContext — nil only when
	// the Auth middleware didn't run).
	//
	// The actor's uuid, not their email — the whole point of the migration
	// is that this column stops holding addresses. Subject is what the
	// verified token carries and what authorisation is already keyed on.
	var actor string
	if info := auth.FromContext(r.Context()); info != nil {
		actor = info.Subject
	}

	u, err := d.Users.Upsert(r.Context(), dirUser.UUID, actor)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}
	// The response still needs a name — the caller just picked this person
	// and expects to see who — even though nothing was stored. Always the
	// identity directory's answer: dirUser is never nil past this point.
	u.Email = req.Email
	u.DisplayName = dirUser.DisplayName
	response.WriteJSONValue(w, http.StatusOK, u)
}

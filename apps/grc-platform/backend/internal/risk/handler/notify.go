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
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// notifyTimeout caps the whole background notification: the Compliance Entity
// lookups plus an email send that retries once. It has to outlast that retry —
// sized too tightly, it would cancel the second attempt before it could land,
// which is the exact failure the retry exists to fix. The individual HTTP
// clients all have their own timeouts, so this is a backstop against a stuck
// goroutine rather than the mechanism that bounds any single call.
const notifyTimeout = 2 * time.Minute

// notifyConcurrency bounds how many notifications can be actively doing work
// (entity lookups + email send) at once, across every call site sharing
// notifyRiskEvent below — workflow transitions, plan completion, and the daily
// escalation job alike. Without it, a large batch fans out one goroutine's
// worth of concurrent entity/email-service load per risk; the realistic case
// is the escalation job's first sweep after a backlog builds up (e.g. the job
// was down, or this ships with overdue risks already waiting). A goroutine
// still spawns immediately either way — this only gates the work inside it —
// so notifyRiskEvent's callers are never blocked by it.
const notifyConcurrency = 5

var notifySem = make(chan struct{}, notifyConcurrency)

// notifyRiskEvent emails everyone in recipientUserIDs about ev.
//
// Best-effort and detached: the transition it reports is already committed by
// the time this runs, so a failure here is logged and swallowed rather than
// turning a successful workflow action into an error for the caller. It runs on
// a context of its own because email-service cold starts take tens of seconds
// while the request context is cancelled the moment the handler returns —
// running inline would both stall the response and cut the send off midway.
//
// Duplicate and zero ids are tolerated: ids are de-duplicated, and a recipient
// with no email on file is skipped rather than failing the whole send.
func (d *Deps) notifyRiskEvent(ev emailer.RiskEvent, riskID int, recipientUserIDs []int, actor, comment string) {
	go func() {
		// net/http recovers a panic raised on the request path; a bare
		// goroutine has no such net, so an unguarded panic here would take the
		// whole process down instead of failing one notification.
		defer func() {
			if p := recover(); p != nil {
				slog.Error("risk notification: panic", "event", ev, "riskId", riskID, "panic", p)
			}
		}()
		// Wait for a slot before doing any work. Not counted against
		// notifyTimeout below — that timeout bounds the entity/email work
		// itself, not queueing time behind a large batch, which is expected
		// backpressure rather than a stuck goroutine.
		notifySem <- struct{}{}
		defer func() { <-notifySem }()
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		// Error discarded: every failure path below already logs with the
		// risk id at Warn/Error, which is all a fire-and-forget caller can
		// act on. A caller that needs to know the outcome wants
		// sendRiskEventSync instead.
		_ = d.sendRiskEvent(ctx, ev, riskID, recipientUserIDs, actor, comment)
	}()
}

// sendRiskEventSync is sendRiskEvent's counterpart for a caller that runs the
// work inline rather than detached and needs to know whether it succeeded —
// currently only NotifyEscalationSync, for the daily escalation job. Goes
// through the same notifySem bound and gets its own notifyTimeout budget,
// same as the async path; the panic recovery here converts to a returned
// error instead of only logging, since a caller running through this path
// still wants to keep processing the rest of its own batch.
func (d *Deps) sendRiskEventSync(ctx context.Context, ev emailer.RiskEvent, riskID int, recipientUserIDs []int, actor, comment string) (err error) {
	notifySem <- struct{}{}
	defer func() { <-notifySem }()
	defer func() {
		if p := recover(); p != nil {
			slog.Error("risk notification: panic", "event", ev, "riskId", riskID, "panic", p)
			err = fmt.Errorf("risk notification panic: %v", p)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()
	return d.sendRiskEvent(ctx, ev, riskID, recipientUserIDs, actor, comment)
}

// sendRiskEvent resolves recipients and the risk detail, then sends. Split out
// from notifyRiskEvent so the work is callable both detached (via the
// goroutine above) and synchronously (via sendRiskEventSync).
func (d *Deps) sendRiskEvent(ctx context.Context, ev emailer.RiskEvent, riskID int, recipientUserIDs []int, actor, comment string) error {
	seen := make(map[int]bool, len(recipientUserIDs))
	emails := make([]string, 0, len(recipientUserIDs))
	for _, id := range recipientUserIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		u, err := d.Users.GetByID(ctx, id)
		if err != nil {
			slog.Warn("risk notification: failed to resolve recipient",
				"event", ev, "riskId", riskID, "userId", id, "err", err)
			continue
		}
		if u == nil {
			slog.Warn("risk notification: recipient has no user row",
				"event", ev, "riskId", riskID, "userId", id)
			continue
		}
		// A deliverable address, from the directory — the only source now that
		// the platform is removing stored emails. This is the reason the
		// directory cache serves stale values on failure: a recipient with no
		// address is dropped here, and a send with none left does not happen
		// at all — so a directory blip would otherwise turn into silently
		// unsent notifications.
		_, email := d.resolvePerson(ctx, u.UUID)
		if email == "" {
			slog.Warn("risk notification: recipient has no email on file",
				"event", ev, "riskId", riskID, "userId", id)
			continue
		}
		emails = append(emails, email)
	}
	if len(emails) == 0 {
		slog.Warn("risk notification: no deliverable recipients", "event", ev, "riskId", riskID)
		return fmt.Errorf("no deliverable recipients")
	}

	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("risk notification: failed to load risk detail", "event", ev, "riskId", riskID, "err", err)
		return fmt.Errorf("load risk detail: %w", err)
	}
	riskLevel := ""
	if detail.GrossScore != nil {
		riskLevel = detail.GrossScore.RiskLevel
	}

	if err := d.Email.SendRiskEvent(ctx, ev, emails, emailer.RiskEventInfo{
		RiskCode:       detail.RiskCode,
		RiskTitle:      detail.RiskTitle,
		SourceRegister: detail.SourceRegisterName,
		RiskLevel:      riskLevel,
		Actor:          d.describeActor(ctx, actor),
		Comment:        comment,
		People:         d.peopleForEvent(ctx, ev, riskID, detail),
		DetailURL:      fmt.Sprintf("%s/risk/registers?riskId=%d", d.FrontendBaseURL, riskID),
	}); err != nil {
		slog.Warn("risk notification: send failed", "event", ev, "riskId", riskID, "recipients", len(emails), "userIds", recipientUserIDs, "err", err)
		return fmt.Errorf("send: %w", err)
	}
	slog.Info("risk notification sent", "event", ev, "riskId", riskID, "recipients", len(emails), "userIds", recipientUserIDs)
	return nil
}

// peopleForEvent resolves the roles ev's template will actually render into the
// people filling them, formatted "Display Name (email)".
//
// Driven by emailer.ActionRoles rather than resolving every role on every risk:
// most events name one role, and each unlisted role saved is a Compliance
// Entity round trip avoided. A role that resolves to nobody is simply absent,
// and the template drops the row.
func (d *Deps) peopleForEvent(ctx context.Context, ev emailer.RiskEvent, riskID int, detail *model.RiskDetail) map[string][]string {
	roles := emailer.ActionRoles(ev)
	if len(roles) == 0 {
		return nil
	}
	people := make(map[string][]string, len(roles))
	for _, role := range roles {
		switch role {
		case emailer.RoleRiskAssigner:
			if p := d.personLabel(ctx, detail.AssignerID); p != "" {
				people[role] = []string{p}
			}
		case emailer.RoleRiskOwner:
			if p := d.personLabel(ctx, detail.OwnerID); p != "" {
				people[role] = []string{p}
			}
		case emailer.RoleManagementApprover:
			if p := d.personLabel(ctx, detail.ManagementApproverID); p != "" {
				people[role] = []string{p}
			}
		case emailer.RoleActionOwner:
			// From the plan list, not detail.action_plan — that field only ever
			// embeds the STANDARD plan, and a risk may have several plans with
			// different owners.
			plans, err := d.ActionPlan.List(ctx, riskID)
			if err != nil {
				slog.Warn("risk notification: failed to list plans for roles", "riskId", riskID, "err", err)
				continue
			}
			seen := map[int]bool{}
			for _, pl := range plans {
				if pl.ActionOwnerID == nil || seen[*pl.ActionOwnerID] {
					continue
				}
				seen[*pl.ActionOwnerID] = true
				if p := d.personLabel(ctx, *pl.ActionOwnerID); p != "" {
					people[role] = append(people[role], p)
				}
			}
		}
	}
	return people
}

// personLabel renders a platform user as "Display Name (email)", or "" when
// they can't be resolved. Same shape as describeActor, but keyed by id rather
// than uuid — the risk row stores ids.
func (d *Deps) personLabel(ctx context.Context, userID int) string {
	if userID <= 0 {
		return ""
	}
	u, err := d.Users.GetByID(ctx, userID)
	if err != nil || u == nil {
		return ""
	}
	name, email := d.resolvePerson(ctx, u.UUID)
	if name == "" {
		return email
	}
	if email == "" {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, email)
}

// resolvePerson answers who a uuid actually is — the identity directory is
// the only source, now that the platform is removing stored names and
// emails from the database entirely. Empty when the directory is
// unconfigured, the uuid is blank, or the person is unresolvable; callers
// must not fall back to a stored value, since none exists to fall back to.
//
// This is deliberately safe to call on someone who was never validated
// resolvable: every role a risk actually names (Owner, Assigner, Management
// Approver, Action Owner) is now only reachable through a picker or resolve
// flow that already required a successful directory lookup — see
// candidates.go and resolve.go — so a miss here should be rare, not the
// normal case this function is designed around.
func (d *Deps) resolvePerson(ctx context.Context, uuid string) (name, email string) {
	if d.Directory == nil || uuid == "" {
		return "", ""
	}
	p, ok := d.Directory.Lookup(ctx, uuid)
	if !ok {
		return "", ""
	}
	return p.DisplayName, p.Email
}

// describeActor renders the person who triggered a notification as
// "Display Name (email@wso2.com)".
//
// Falls back to the raw uuid when nothing resolves — deliberately, and
// distinct from the identity-gated roles above: the actor already
// authenticated and acted (they are not being newly named on a risk), and
// the daily escalation job's "system" sentinel actor depends on exactly this
// fallback (no `user` row will ever exist for it — see escalation_job.go's
// escalatedBy). A notification is never worth failing over a cosmetic
// lookup, so every path here returns something sendable.
func (d *Deps) describeActor(ctx context.Context, actorUUID string) string {
	actorUUID = strings.TrimSpace(actorUUID)
	if actorUUID == "" {
		return ""
	}
	name, email := d.resolvePerson(ctx, actorUUID)
	switch {
	case name != "" && email != "":
		return fmt.Sprintf("%s (%s)", name, email)
	case name != "":
		return name
	case email != "":
		return email
	default:
		return actorUUID
	}
}

// notifyComplianceAdmins emails everyone who holds RISK_COMPLIANCE_APPROVE in
// registerID — GLOBAL, or scoped to that specific register — at one of three
// lifecycle points: risk created, risk reaching PENDING_COMPLIANCE_REVIEW, and
// risk reaching PENDING_COMPLIANCE_CLOSURE.
//
// Resolved the same way candidates.go populates the Risk Owner/Management
// Approver pickers — one query against user_role_grant — rather than the
// retired RISK_COMPLIANCE_ADMIN_GROUP Asgardeo group, so "who gets notified"
// and "who could actually approve this" can never drift apart.
//
// Detached and best-effort, mirroring notifyRiskEvent: the candidate lookup is
// itself a Compliance Entity round trip, so it runs inside the same
// goroutine/semaphore/timeout envelope rather than blocking the caller, whose
// transition is already committed by the time this runs. A register nobody
// currently holds the privilege in logs a warning rather than failing that
// already-committed transition.
func (d *Deps) notifyComplianceAdmins(ev emailer.RiskEvent, riskID, registerID int, actor, comment string) {
	if d.Grants == nil {
		// Local dev: no privilege store configured, so no grants exist to
		// resolve a recipient list from.
		return
	}
	go func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("compliance-admin notification: panic", "event", ev, "riskId", riskID, "panic", p)
			}
		}()
		notifySem <- struct{}{}
		defer func() { <-notifySem }()
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()

		candidates, err := d.Grants.Candidates(ctx, privilege.ComplianceApproveRisk, []int{registerID})
		if err != nil {
			slog.Warn("compliance-admin notification: failed to resolve recipients",
				"event", ev, "riskId", riskID, "err", err)
			return
		}
		if len(candidates) == 0 {
			slog.Warn("compliance-admin notification: nobody holds RISK_COMPLIANCE_APPROVE in this register",
				"event", ev, "riskId", riskID, "registerId", registerID)
			return
		}
		ids := make([]int, len(candidates))
		for i, c := range candidates {
			ids[i] = c.ID
		}
		_ = d.sendRiskEvent(ctx, ev, riskID, ids, actor, comment)
	}()
}

// notifyEscalationLeads is a deliberate no-op, for the same reason as
// notifyComplianceAdmins: the recipients were explicitly deferred.
//
// The assigner's and action owner's line managers are already resolved from the
// HR entity and frozen on the escalation row at escalation time, so the data is
// there — only the send is withheld. That ordering is intentional: resolving
// them later would risk a reorg changing who a historical escalation belonged
// to.
//
// TODO: send to assigner_lead_email / action_owner_lead_email on the risk's
// open escalation once it is decided that leads should be emailed.
func notifyEscalationLeads(riskID int) {
	slog.Info("escalation lead notification suppressed (not yet wired)", "riskId", riskID)
}

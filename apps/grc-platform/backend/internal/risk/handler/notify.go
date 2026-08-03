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
)

// notifyTimeout caps the whole background notification: the Compliance Entity
// lookups plus an email send that retries once. It has to outlast that retry —
// sized too tightly, it would cancel the second attempt before it could land,
// which is the exact failure the retry exists to fix. The individual HTTP
// clients all have their own timeouts, so this is a backstop against a stuck
// goroutine rather than the mechanism that bounds any single call.
const notifyTimeout = 2 * time.Minute

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
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		d.sendRiskEvent(ctx, ev, riskID, recipientUserIDs, actor, comment)
	}()
}

// sendRiskEvent resolves recipients and the risk detail, then sends. Split out
// from notifyRiskEvent so the work is callable synchronously in a test without
// the goroutine.
func (d *Deps) sendRiskEvent(ctx context.Context, ev emailer.RiskEvent, riskID int, recipientUserIDs []int, actor, comment string) {
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
		if u == nil || u.Email == "" {
			slog.Warn("risk notification: recipient has no email on file",
				"event", ev, "riskId", riskID, "userId", id)
			continue
		}
		emails = append(emails, u.Email)
	}
	if len(emails) == 0 {
		slog.Warn("risk notification: no deliverable recipients", "event", ev, "riskId", riskID)
		return
	}

	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("risk notification: failed to load risk detail", "event", ev, "riskId", riskID, "err", err)
		return
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
		slog.Warn("risk notification: send failed", "event", ev, "riskId", riskID, "to", emails, "err", err)
		return
	}
	slog.Info("risk notification sent", "event", ev, "riskId", riskID, "to", emails)
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
// than email — the risk row stores ids.
func (d *Deps) personLabel(ctx context.Context, userID int) string {
	if userID <= 0 {
		return ""
	}
	u, err := d.Users.GetByID(ctx, userID)
	if err != nil || u == nil {
		return ""
	}
	name := strings.TrimSpace(u.DisplayName)
	if name == "" {
		return u.Email
	}
	if u.Email == "" {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, u.Email)
}

// describeActor renders the person who triggered a notification as
// "Display Name (email@wso2.com)". Handlers only have the caller's email (it is
// what the JWT carries), and an email alone reads poorly in a notification —
// but it is also the unambiguous identifier, so both are shown rather than
// swapping one for the other.
//
// Degrades to the bare email whenever the name can't be resolved: the caller
// may have no platform user row yet, the lookup may fail, or the row may have
// no display name. A notification is never worth failing over a cosmetic
// lookup, so every one of those paths returns something sendable.
func (d *Deps) describeActor(ctx context.Context, email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	u, err := d.Users.GetByEmail(ctx, email)
	if err != nil {
		slog.Warn("risk notification: failed to resolve actor name", "actor", email, "err", err)
		return email
	}
	if u == nil || strings.TrimSpace(u.DisplayName) == "" {
		return email
	}
	return fmt.Sprintf("%s (%s)", strings.TrimSpace(u.DisplayName), email)
}

// notifyComplianceAdmins is a deliberate no-op.
//
// Three lifecycle points should notify the Compliance Admin role — risk
// created, risk reaching PENDING_COMPLIANCE_REVIEW, and risk reaching
// PENDING_COMPLIANCE_CLOSURE. Unlike every other notification, the recipient is
// a *role* rather than a named individual, so it would fan out to everyone
// holding it. Who should actually receive these has not been decided, and
// sending to the whole role in the meantime would be worse than sending
// nothing.
//
// It is wired into all three call sites and logs, so the trigger points are
// exercised and observable while testing. Turning it on later means resolving
// the role to a recipient list here — no call-site changes.
//
// TODO: resolve the Compliance Admin role to recipients (Asgardeo group
// membership via internal/scim, the same mechanism /management-approvers uses)
// and send, once it is decided who should be on that list.
func notifyComplianceAdmins(ev emailer.RiskEvent, riskID int) {
	slog.Info("compliance-admin notification suppressed (not yet wired)",
		"event", ev, "riskId", riskID)
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

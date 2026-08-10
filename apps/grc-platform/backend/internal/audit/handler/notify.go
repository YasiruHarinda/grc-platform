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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
)

// notifyTimeout caps the whole background notification: the user lookup plus
// an email send that retries once. Same value/reasoning as risk's notify.go.
const notifyTimeout = 2 * time.Minute

// notifyConcurrency bounds how many audit notifications can be actively doing
// work at once. This is the audit module's own bound — notifySem below is
// package-private to internal/risk/handler, so it can't be shared across
// packages; same reasoning as risk's own comment on notifyConcurrency.
const notifyConcurrency = 5

var notifySem = make(chan struct{}, notifyConcurrency)

// notificationLogItem is one audit_notification row to write after a
// successful send — kept separate from emailer.AuditEventItem (display-only
// strings) so the emailer package stays free of any audit-module DB-log
// concept. Exactly one of ControlID/PopulationID should be set, matching the
// table's own convention. A slice of these must be index-aligned with the
// accompanying emailer.AuditEventInfo.Items: the de-dup key is per-item even
// when several items are emailed together as one digest, and a digest can
// span more than one audit (an owner's reminder items aren't confined to a
// single audit), so AuditID lives per item rather than once for the batch.
type notificationLogItem struct {
	AuditID         *int
	Type            string // audit_notification.type enum value
	ControlID       *int
	PopulationID    *int
	DueDateSnapshot *string
}

// notifyAuditEvent emails ownerUserID about info.Items, detached from the
// request context, fire-and-forget.
//
// Best-effort and detached: the transition it reports is already committed by
// the time this runs, so a failure here is logged and swallowed rather than
// turning a successful workflow action into an error for the caller. It runs
// on a context of its own because email-service cold starts take tens of
// seconds while the request context is cancelled the moment the handler
// returns — running inline would both stall the response and cut the send
// off midway.
func (d *Deps) notifyAuditEvent(ev emailer.AuditEvent, ownerUserID int, info emailer.AuditEventInfo, logItems []notificationLogItem) {
	go func() {
		// net/http recovers a panic raised on the request path; a bare
		// goroutine has no such net, so an unguarded panic here would take
		// the whole process down instead of failing one notification.
		defer func() {
			if p := recover(); p != nil {
				slog.Error("audit notification: panic", "event", ev, "ownerId", ownerUserID, "panic", p)
			}
		}()
		notifySem <- struct{}{}
		defer func() { <-notifySem }()
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		_ = d.sendAuditEvent(ctx, ev, ownerUserID, info, logItems)
	}()
}

// sendAuditEventSync is notifyAuditEvent's synchronous counterpart, used only
// by the daily reminder job (internal/audit/job): the job needs to know
// per-recipient success to count run-level failures, and a fire-and-forget
// send there could silently under-deliver a whole day's digest run.
func (d *Deps) sendAuditEventSync(ctx context.Context, ev emailer.AuditEvent, ownerUserID int, info emailer.AuditEventInfo, logItems []notificationLogItem) (err error) {
	notifySem <- struct{}{}
	defer func() { <-notifySem }()
	defer func() {
		if p := recover(); p != nil {
			slog.Error("audit notification: panic", "event", ev, "ownerId", ownerUserID, "panic", p)
			err = fmt.Errorf("audit notification panic: %v", p)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()
	return d.sendAuditEvent(ctx, ev, ownerUserID, info, logItems)
}

// sendAuditEvent resolves ownerUserID (skipping — not an error — if they're
// not found, have no email, or are not ACTIVE, per the cross-cutting
// active-users-only rule), sends, and best-effort logs one audit_notification
// row per logItems entry.
func (d *Deps) sendAuditEvent(ctx context.Context, ev emailer.AuditEvent, ownerUserID int, info emailer.AuditEventInfo, logItems []notificationLogItem) error {
	if ownerUserID <= 0 {
		return nil
	}
	u, err := d.Users.GetByID(ctx, ownerUserID)
	if err != nil {
		slog.Warn("audit notification: failed to resolve recipient", "event", ev, "ownerId", ownerUserID, "err", err)
		return fmt.Errorf("resolve recipient: %w", err)
	}
	if u == nil || u.Email == "" {
		slog.Warn("audit notification: recipient has no email on file", "event", ev, "ownerId", ownerUserID)
		return nil
	}
	if u.Status != "" && u.Status != "ACTIVE" {
		slog.Info("audit notification: recipient not active, skipping", "event", ev, "ownerId", ownerUserID, "status", u.Status)
		return nil
	}

	if err := d.Email.SendAuditEvent(ctx, ev, u.Email, info); err != nil {
		slog.Warn("audit notification: send failed", "event", ev, "ownerId", ownerUserID, "err", err)
		return fmt.Errorf("send: %w", err)
	}
	slog.Info("audit notification sent", "event", ev, "ownerId", ownerUserID, "items", len(info.Items))

	d.logSends(ctx, ownerUserID, logItems)
	return nil
}

// logSends best-effort logs one audit_notification row per item. A logging
// failure is logged and swallowed: the email already sent successfully, and
// failing the whole notification over an audit-trail write would be worse
// than a missing log row.
func (d *Deps) logSends(ctx context.Context, recipientUserID int, logItems []notificationLogItem) {
	if d.Notification == nil {
		return
	}
	for _, item := range logItems {
		entry := model.NotificationLogEntry{
			RecipientID:     recipientUserID,
			AuditID:         item.AuditID,
			ControlID:       item.ControlID,
			PopulationID:    item.PopulationID,
			Type:            item.Type,
			DueDateSnapshot: item.DueDateSnapshot,
		}
		if err := d.Notification.Create(ctx, entry); err != nil {
			slog.Warn("audit notification: failed to log send", "recipientId", recipientUserID, "type", item.Type, "err", err)
		}
	}
}

// describeActor renders the person who triggered a notification as
// "Display Name (email)". Handlers only have the caller's email (it is what
// the JWT carries), and an email alone reads poorly in a notification — but
// it is also the unambiguous identifier, so both are shown rather than
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
		slog.Warn("audit notification: failed to resolve actor name", "actor", email, "err", err)
		return email
	}
	if u == nil || strings.TrimSpace(u.DisplayName) == "" {
		return email
	}
	return fmt.Sprintf("%s (%s)", strings.TrimSpace(u.DisplayName), email)
}

// detailURL builds the "View in Audit Hub" link for auditID — every audit
// notification links to the audit detail page; the frontend has no
// deep-link support for opening one control's drawer directly.
func (d *Deps) detailURL(auditID int) string {
	return fmt.Sprintf("%s/audit/audits/%d", d.FrontendBaseURL, auditID)
}

// auditName resolves auditID to its display name for the "Audit: ..." line
// in single-audit-scoped notification emails (owner-assigned, resubmission,
// sample-submitted). Not used by the work-queue or reminder-digest emails —
// those can span more than one audit, so no single name applies.
//
// Best-effort, same contract as describeActor: a lookup failure or missing
// audit degrades to "" (the template omits the line entirely) rather than
// failing the notification over a cosmetic field.
func (d *Deps) auditName(ctx context.Context, auditID int) string {
	a, err := d.Audit.GetByID(ctx, auditID)
	if err != nil {
		slog.Warn("audit notification: failed to resolve audit name", "auditId", auditID, "err", err)
		return ""
	}
	if a == nil {
		return ""
	}
	return a.Name
}

// SendReminderDigestSync sends one owner's full daily due-date reminder
// digest — one combined email covering every control/population item due
// across all three tiers — and logs one audit_notification row per item on
// success. Used only by the reminder job (internal/audit/job), which needs
// to know per-recipient success to count a run's totals; wired to
// job.ReminderJob at startup exactly as risk's NotifyEscalationSync is
// wired to its escalation job (see cmd/server/main.go).
func (d *Deps) SendReminderDigestSync(ctx context.Context, ownerUserID int, items []model.ReminderItem) error {
	if len(items) == 0 {
		return nil
	}
	emailItems := make([]emailer.AuditEventItem, 0, len(items))
	logItems := make([]notificationLogItem, 0, len(items))
	for _, it := range items {
		emailItems = append(emailItems, emailer.AuditEventItem{
			ControlNumber: it.ControlNumber,
			Description:   it.Description,
			DueDate:       it.DueDate,
			Tier:          it.Tier,
			Kind:          it.Kind,
		})
		auditID, dedupSnapshot := it.AuditID, it.DedupSnapshot
		logItems = append(logItems, notificationLogItem{
			AuditID:         &auditID,
			Type:            it.Type,
			ControlID:       it.ControlID,
			PopulationID:    it.PopulationID,
			DueDateSnapshot: &dedupSnapshot,
		})
	}

	// Pick the event constant that best fits the batch's subject line: the
	// most urgent tier present (overdue > due-in-5 > due-in-10), since an
	// owner with items across multiple tiers still gets exactly one email.
	ev := emailer.AuditEventReminderDue10
	for _, it := range items {
		switch it.Type {
		case "REMINDER_OVERDUE":
			ev = emailer.AuditEventReminderOverdue
		case "REMINDER_DUE_5":
			if ev != emailer.AuditEventReminderOverdue {
				ev = emailer.AuditEventReminderDue5
			}
		}
	}

	// A digest can span more than one audit (an owner's reminder items aren't
	// confined to a single audit), so this links to the dashboard rather than
	// any one audit's detail page.
	info := emailer.AuditEventInfo{
		DetailURL: d.FrontendBaseURL + "/audit/dashboard",
		Items:     emailItems,
	}
	return d.sendAuditEventSync(ctx, ev, ownerUserID, info, logItems)
}

// notifyResubmission handles the resubmission-needed event for all four
// reject transitions (population/evidence x internal-review/validation),
// routed through the shared decideRound engine in review.go. control is the
// pre-transition control (still carries the population's own owner via
// PopulationOwnerID/PopulationID); controlStatus is the target status
// decideRound just set, which determines whether the population's owner or
// the control's owner gets notified.
func (d *Deps) notifyResubmission(ctx context.Context, control *model.AuditControl, controlStatus string, comment *string, actor string) {
	var (
		ownerID      *int
		phase        string
		logControlID *int
		logPopID     *int
	)
	switch controlStatus {
	case "POPULATION_PENDING", "POPULATION_NEED_CLARIFICATION":
		ownerID, phase = control.PopulationOwnerID, "Population"
		logPopID = control.PopulationID
	case "EVIDENCE_PENDING", "EVIDENCE_NEED_CLARIFICATION":
		ownerID, phase = control.OwnerID, "Evidence"
		logControlID = &control.ID
	default:
		return
	}
	if ownerID == nil {
		return // unassigned — silently skip, per the cross-cutting rule
	}

	commentText := ""
	if comment != nil {
		commentText = *comment
	}
	info := emailer.AuditEventInfo{
		AuditName: d.auditName(ctx, control.AuditID),
		Actor:     d.describeActor(ctx, actor),
		Comment:   commentText,
		DetailURL: d.detailURL(control.AuditID),
		Items: []emailer.AuditEventItem{{
			ControlNumber: control.ControlNumber,
			Description:   phase + " phase: " + control.Description,
		}},
	}
	logItems := []notificationLogItem{{
		AuditID:      &control.AuditID,
		Type:         "RESUBMISSION_NEEDED",
		ControlID:    logControlID,
		PopulationID: logPopID,
	}}
	d.notifyAuditEvent(emailer.AuditEventResubmissionNeeded, *ownerID, info, logItems)
}

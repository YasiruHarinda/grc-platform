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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
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
		_ = d.sendAuditEvent(ctx, ev, ownerUserID, info, logItems, false)
	}()
}

// sendAuditEventSync is notifyAuditEvent's synchronous counterpart, used only
// by the daily reminder job (internal/audit/job): the job needs to know
// per-recipient success to count run-level failures, and a fire-and-forget
// send there could silently under-deliver a whole day's digest run.
//
// skipLog is true only for the reminder job's own call
// (SendReminderDigestSync): its items are already logged by the job's own
// Claim, before the send was even attempted (see
// docs/new/Reminder-Notification-Atomic-Claim-Design.md) — logging again here
// would insert a second row and collide with uq_notif_reminder_dedup.
func (d *Deps) sendAuditEventSync(ctx context.Context, ev emailer.AuditEvent, ownerUserID int, info emailer.AuditEventInfo, logItems []notificationLogItem, skipLog bool) (err error) {
	select {
	case notifySem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-notifySem }()
	defer func() {
		if p := recover(); p != nil {
			slog.Error("audit notification: panic", "event", ev, "ownerId", ownerUserID, "panic", p)
			err = fmt.Errorf("audit notification panic: %v", p)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()
	return d.sendAuditEvent(ctx, ev, ownerUserID, info, logItems, skipLog)
}

// sendAuditEvent resolves ownerUserID (skipping — not an error — if they're
// not found, have no email, or are not ACTIVE, per the cross-cutting
// active-users-only rule), sends, and — unless skipLog — best-effort logs one
// audit_notification row per logItems entry.
func (d *Deps) sendAuditEvent(ctx context.Context, ev emailer.AuditEvent, ownerUserID int, info emailer.AuditEventInfo, logItems []notificationLogItem, skipLog bool) error {
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

	if !skipLog {
		d.logSends(ctx, ownerUserID, logItems)
	}
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

// detailURL builds the "View in Audit Hub" link for auditID alone — used only
// where no single control applies (the reminder digest, which can span many
// controls across many audits). Every other notification is about one
// control and should use controlDetailURL instead.
func (d *Deps) detailURL(auditID int) string {
	return fmt.Sprintf("%s/audit/audits/%d", d.FrontendBaseURL, auditID)
}

// controlDetailURL builds a "View in Audit Hub" link that deep-links straight
// to one control's drawer, via the ?control= query param the audit detail
// page already reads on load (AuditDetailPage.tsx — the same param
// WorkQueue/BlockerList use to jump from the dashboard).
func (d *Deps) controlDetailURL(auditID, controlID int) string {
	return fmt.Sprintf("%s/audit/audits/%d?control=%d", d.FrontendBaseURL, auditID, controlID)
}

// singleControlID returns the one control ID in ids and true, only when ids
// contains exactly one entry — used to decide whether a batched
// owner-assigned email (which can span more than one control) can still link
// straight to a control instead of falling back to the audit's control list.
func singleControlID(ids map[int]bool) (int, bool) {
	if len(ids) != 1 {
		return 0, false
	}
	for id := range ids {
		return id, true
	}
	return 0, false
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
// across all three tiers. Used only by the reminder job (internal/audit/job),
// which needs to know per-recipient success to count a run's totals, and to
// know whether to release each item's claim on failure; wired to
// job.ReminderJob at startup exactly as risk's NotifyEscalationSync is wired
// to its escalation job (see cmd/server/main.go).
//
// Unlike every other notify path, this does NOT write audit_notification rows
// on success — each item was already logged by the job's own atomic Claim,
// before this function was even called (see
// docs/new/Reminder-Notification-Atomic-Claim-Design.md). Logging again here
// would insert a second row per item and collide with uq_notif_reminder_dedup.
func (d *Deps) SendReminderDigestSync(ctx context.Context, ownerUserID int, items []model.ReminderItem) error {
	if len(items) == 0 {
		return nil
	}
	emailItems := make([]emailer.AuditEventItem, 0, len(items))
	for _, it := range items {
		emailItems = append(emailItems, emailer.AuditEventItem{
			ControlNumber:   it.ControlNumber,
			Description:     it.Description,
			DueDate:         it.DueDate,
			Tier:            it.Tier,
			RequirementType: it.RequirementType,
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
		// Only this digest mixes tiers in one email (an owner's due-in-10,
		// due-in-5, and overdue items all together) — Status is the only
		// place that distinguishes them, so only here is it worth showing.
		ShowStatus: true,
	}
	return d.sendAuditEventSync(ctx, ev, ownerUserID, info, nil, true)
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
		ownerID         *int
		requirementType string
		dueDate         string
		logControlID    *int
		logPopID        *int
	)
	switch controlStatus {
	case "POPULATION_PENDING", "POPULATION_NEED_CLARIFICATION":
		ownerID = control.PopulationOwnerID
		requirementType = "Population Requirement"
		dueDate = derefString(control.PopulationDueDate)
		logPopID = control.PopulationID
	case "EVIDENCE_PENDING", "EVIDENCE_NEED_CLARIFICATION":
		ownerID = control.OwnerID
		requirementType = "Evidence Requirement"
		dueDate = derefString(control.DueDate)
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
		DetailURL: d.controlDetailURL(control.AuditID, control.ID),
		Items: []emailer.AuditEventItem{{
			ControlNumber:   control.ControlNumber,
			Description:     control.Description,
			DueDate:         dueDate,
			RequirementType: requirementType,
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

// notifyControlStatusReached fires whatever email (if any) is mapped to
// newStatus, regardless of which endpoint produced the transition —
// decideRound's approve branch, submitEvidence, submitPopulation, or an
// admin status override. Statuses with no mapping are a no-op; reject has its
// own separate notifyResubmission path, untouched by this.
func (d *Deps) notifyControlStatusReached(ctx context.Context, control *model.AuditControl, newStatus, actor string) {
	switch newStatus {
	case "EVIDENCE_INTERNAL_REVIEW":
		d.notifyAdmins(ctx, emailer.AuditEventEvidenceInternalReview, control, actor, "EVIDENCE_INTERNAL_REVIEW", "Evidence Requirement", derefString(control.DueDate))
	case "POPULATION_INTERNAL_REVIEW":
		d.notifyAdmins(ctx, emailer.AuditEventPopulationInternalReview, control, actor, "POPULATION_INTERNAL_REVIEW", "Population Requirement", derefString(control.PopulationDueDate))
	case "EVIDENCE_UNDER_VALIDATION":
		d.notifyAuditor(ctx, emailer.AuditEventEvidenceUnderValidation, control, actor, "EVIDENCE_UNDER_VALIDATION", "Evidence Requirement", derefString(control.DueDate))
	case "POPULATION_UNDER_VALIDATION":
		d.notifyAuditor(ctx, emailer.AuditEventPopulationUnderValidation, control, actor, "POPULATION_UNDER_VALIDATION", "Population Requirement", derefString(control.PopulationDueDate))
	case "POPULATION_COMPLETE":
		d.notifyAuditor(ctx, emailer.AuditEventPopulationCompleteSampleNeeded, control, actor, "POPULATION_COMPLETE_SAMPLE_NEEDED", "Population Requirement", derefString(control.PopulationDueDate))
	case "COMPLETE":
		d.notifyControlComplete(ctx, control, actor)
	}
}

// controlEventInfo builds the one-item AuditEventInfo shared by
// notifyControlStatusReached's recipients. Comment is deliberately never set
// here — none of these six events carry one.
func (d *Deps) controlEventInfo(ctx context.Context, control *model.AuditControl, actor, requirementType, dueDate string) emailer.AuditEventInfo {
	return emailer.AuditEventInfo{
		AuditName: d.auditName(ctx, control.AuditID),
		Actor:     d.describeActor(ctx, actor),
		DetailURL: d.controlDetailURL(control.AuditID, control.ID),
		Items: []emailer.AuditEventItem{{
			ControlNumber:   control.ControlNumber,
			Description:     control.Description,
			DueDate:         dueDate,
			RequirementType: requirementType,
		}},
	}
}

// resolveAdminIDs returns the platform user IDs of every active Audit
// Compliance Admin (AUDIT_MANAGE_CONTROLS is unique to that role). Best-effort:
// a Candidates lookup failure is logged and swallowed, returning nil, same
// contract as every other notify path here.
func (d *Deps) resolveAdminIDs(ctx context.Context, ev emailer.AuditEvent) []int {
	if d.Grants == nil {
		return nil
	}
	admins, err := d.Grants.Candidates(ctx, privilege.ManageControls, nil)
	if err != nil {
		slog.Warn("audit notification: failed to resolve admin recipients", "event", ev, "err", err)
		return nil
	}
	ids := make([]int, 0, len(admins))
	for _, admin := range admins {
		ids = append(ids, admin.ID)
	}
	return ids
}

// notifyAdmins emails every admin about a control reaching a status they're
// named recipients for.
func (d *Deps) notifyAdmins(ctx context.Context, ev emailer.AuditEvent, control *model.AuditControl, actor, logType, requirementType, dueDate string) {
	info := d.controlEventInfo(ctx, control, actor, requirementType, dueDate)
	for _, id := range d.resolveAdminIDs(ctx, ev) {
		logItems := []notificationLogItem{{AuditID: &control.AuditID, Type: logType, ControlID: &control.ID}}
		d.notifyAuditEvent(ev, id, info, logItems)
	}
}

// notifyAuditor emails the control's assigned auditor (control.AuditorID).
// Silently skipped when no auditor is assigned, matching the cross-cutting
// "unassigned recipient = skip" rule notifyResubmission already follows.
func (d *Deps) notifyAuditor(ctx context.Context, ev emailer.AuditEvent, control *model.AuditControl, actor, logType, requirementType, dueDate string) {
	if control.AuditorID == nil {
		return
	}
	info := d.controlEventInfo(ctx, control, actor, requirementType, dueDate)
	logItems := []notificationLogItem{{AuditID: &control.AuditID, Type: logType, ControlID: &control.ID}}
	d.notifyAuditEvent(ev, *control.AuditorID, info, logItems)
}

// notifyControlComplete fires when a control reaches COMPLETE: the owner,
// every admin, and (if assigned) the auditor each get their own send — the
// email client has no multi-recipient batching, so this is one call per
// resolved recipient. Recipients are deduplicated by user ID first: the same
// person can hold more than one of these roles on a control (e.g. owner and
// assigned auditor), and must get exactly one email, not one per role.
func (d *Deps) notifyControlComplete(ctx context.Context, control *model.AuditControl, actor string) {
	const logType = "CONTROL_COMPLETE"
	info := d.controlEventInfo(ctx, control, actor, "Evidence Requirement", derefString(control.DueDate))

	recipients := map[int]bool{}
	if control.OwnerID != nil {
		recipients[*control.OwnerID] = true
	}
	if control.AuditorID != nil {
		recipients[*control.AuditorID] = true
	}
	for _, id := range d.resolveAdminIDs(ctx, emailer.AuditEventControlComplete) {
		recipients[id] = true
	}
	for id := range recipients {
		logItems := []notificationLogItem{{AuditID: &control.AuditID, Type: logType, ControlID: &control.ID}}
		d.notifyAuditEvent(emailer.AuditEventControlComplete, id, info, logItems)
	}
}

// notifyCommentAdded emails the control's owner(s), every admin, and — when
// the comment is not internal — the assigned auditor about a new comment. An
// internal comment never reaches the auditor, nor an owner who lacks
// AUDIT_VIEW_INTERNAL_COMMENTS: the same gate the read path (comment.go's
// List) enforces. Without this, an owner assigned to a control who happens to
// hold no audit role would get the internal comment's text emailed to them
// even though the app itself never shows it to them. Admins are exempt from
// the owner check — every role resolveAdminIDs resolves also holds
// AUDIT_VIEW_INTERNAL_COMMENTS (see shared_seed_data.sql). The comment's own
// author never gets a copy of their own comment.
//
// Recipient resolution (internal-viewer/admin Candidates lookups, the actor
// lookup, and controlEventInfo's own DB calls) all run inside the goroutine,
// detached from the request context, for the same reason notifyAuditEvent
// runs detached: it must survive past the handler returning, and addComment
// must not block the response on it.
func (d *Deps) notifyCommentAdded(reqCtx context.Context, control *model.AuditControl, comment *model.AuditComment, actor string) {
	go func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("audit notification: panic", "event", emailer.AuditEventCommentAdded, "controlId", control.ID, "panic", p)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()

		// internalViewers is nil for a non-internal comment (no restriction
		// needed). For an internal comment it is the set of user IDs allowed
		// to see it; a lookup failure fails closed to an empty set rather
		// than risking a leak.
		var internalViewers map[int]bool
		if comment.IsInternal {
			internalViewers = map[int]bool{}
			if d.Grants == nil {
				slog.Warn("audit notification: no privilege store, skipping internal-comment owner recipients")
			} else if candidates, err := d.Grants.Candidates(ctx, privilege.ViewInternalComments, nil); err == nil {
				for _, c := range candidates {
					internalViewers[c.ID] = true
				}
			} else {
				slog.Warn("audit notification: failed to resolve internal-comment viewers", "err", err)
			}
		}

		recipients := map[int]bool{}
		addOwner := func(id *int) {
			if id == nil || (internalViewers != nil && !internalViewers[*id]) {
				return
			}
			recipients[*id] = true
		}
		addOwner(control.OwnerID)
		addOwner(control.PopulationOwnerID)
		if !comment.IsInternal && control.AuditorID != nil {
			recipients[*control.AuditorID] = true
		}
		for _, id := range d.resolveAdminIDs(ctx, emailer.AuditEventCommentAdded) {
			recipients[id] = true
		}

		if actorUser, err := d.Users.GetByEmail(ctx, actor); err == nil && actorUser != nil {
			delete(recipients, actorUser.ID)
		}
		if len(recipients) == 0 {
			return
		}

		info := d.controlEventInfo(ctx, control, actor, "", "")
		info.Comment = comment.Content
		for id := range recipients {
			logItems := []notificationLogItem{{AuditID: &control.AuditID, Type: "COMMENT_ADDED", ControlID: &control.ID}}
			d.notifyAuditEvent(emailer.AuditEventCommentAdded, id, info, logItems)
		}
	}()
}

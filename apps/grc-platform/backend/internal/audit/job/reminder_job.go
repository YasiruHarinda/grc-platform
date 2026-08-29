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

// Package job runs the audit module's daily due-date reminder digest.
package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

// auditLister and controlLister are narrow local interfaces, not the full
// repository types — importing those would cycle back through
// internal/audit/handler. Same reasoning as internal/risk/job.
type auditLister interface {
	List(ctx context.Context) ([]*model.Audit, error)
}

type controlLister interface {
	ListAllForReminders(ctx context.Context) ([]*model.AuditControl, error)
}

// claimer is the reminder job's de-dup gate, structurally satisfied by
// NotificationService.Claim/ReleaseClaim without importing that package.
// Claim is the atomic de-dup decision; ReleaseClaim undoes it on a failed send.
type claimer interface {
	Claim(ctx context.Context, recipientID, auditID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (claimed bool, notificationID int64, err error)
	ReleaseClaim(ctx context.Context, notificationID int64) error
}

const (
	// runTimeout bounds a single sweep.
	runTimeout = 30 * time.Minute
	// releaseTimeout bounds a claim release, on its own short deadline instead
	// of the sweep's ctx — a release is often needed because runTimeout just
	// expired, and reusing that same expired ctx would fail immediately.
	releaseTimeout = 10 * time.Second
)

// ReminderJob sweeps every control/population item daily and emails each
// owner one combined digest of everything due in 10 days, due in 5 days, or
// overdue.
type ReminderJob struct {
	audits   auditLister
	controls controlLister
	claim    claimer
	// notify delivers one owner's full daily digest synchronously — a plain
	// function, not a handler dependency, to avoid an import cycle. Wired to
	// handler.Deps.SendReminderDigestSync; doesn't log on success since Claim already did.
	notify func(ctx context.Context, ownerUserID int, items []model.ReminderItem) error
	// admins resolves the admin recipient set once per sweep; notifyAdmin
	// sends one admin's digest of overdue items in one audit. Both nil unless
	// WithAdminAlerts wired them — wired to ReminderAdminIDs / SendOverdueAdminDigestSync.
	admins      func(ctx context.Context) ([]int, error)
	notifyAdmin func(ctx context.Context, adminUserID int, items []model.ReminderItem) error
	// resolveOwnerNames looks up a batch of owner ids in one call, so the
	// escalation resolves each owner once per sweep instead of once per
	// (admin, item) email — wired to handler.Deps.ResolveOwnerNames.
	resolveOwnerNames func(ctx context.Context, ownerIDs []int) map[int]string
	// leads resolves each overdue owner's line-manager email once per sweep;
	// notifyLead sends that owner's overdue items to them. Both nil unless
	// WithLeadAlerts wired them.
	leads      func(ctx context.Context, ownerIDs []int) map[int]string
	notifyLead func(ctx context.Context, ownerUserID int, leadEmail string, items []model.ReminderItem) error
	// running serializes runOnce against itself: Start's daily ticker and the
	// manual-trigger endpoint (handler.reminderJobHandler.run) both end up
	// calling runOnce on this same instance. This guards a same-process
	// overlap cheaply; claimer's DB-level unique constraint is what makes the
	// de-dup correct across processes/replicas too.
	running atomic.Bool
}

// NewReminderJob constructs a ReminderJob.
func NewReminderJob(
	audits auditLister,
	controls controlLister,
	claim claimer,
	notify func(ctx context.Context, ownerUserID int, items []model.ReminderItem) error,
) *ReminderJob {
	return &ReminderJob{audits: audits, controls: controls, claim: claim, notify: notify}
}

// WithAdminAlerts turns on the overdue admin escalation: every admin
// `admins` returns gets one digest email per audit with overdue items.
// Opt-in setter (nil functions skip it) so existing callers stay unchanged.
func (j *ReminderJob) WithAdminAlerts(
	admins func(ctx context.Context) ([]int, error),
	notifyAdmin func(ctx context.Context, adminUserID int, items []model.ReminderItem) error,
	resolveOwnerNames func(ctx context.Context, ownerIDs []int) map[int]string,
) *ReminderJob {
	j.admins = admins
	j.notifyAdmin = notifyAdmin
	j.resolveOwnerNames = resolveOwnerNames
	return j
}

// WithLeadAlerts turns on the overdue lead escalation: each owner's line
// manager gets a digest of that owner's overdue items. Opt-in setter (nil
// functions skip it) so a disabled deployment resolves no leads at all.
func (j *ReminderJob) WithLeadAlerts(
	leads func(ctx context.Context, ownerIDs []int) map[int]string,
	notifyLead func(ctx context.Context, ownerUserID int, leadEmail string, items []model.ReminderItem) error,
) *ReminderJob {
	j.leads = leads
	j.notifyLead = notifyLead
	return j
}

// overdueOnly returns just the overdue entries of an owner's digest — the only
// tier that escalates to a lead.
func overdueOnly(items []model.ReminderItem) []model.ReminderItem {
	out := make([]model.ReminderItem, 0, len(items))
	for _, it := range items {
		if it.Type == "REMINDER_OVERDUE" {
			out = append(out, it)
		}
	}
	return out
}

// hasOverdue answers overdueOnly's question without building the slice, for
// the pre-pass that decides which owners are worth an HR lookup.
func hasOverdue(items []model.ReminderItem) bool {
	for _, it := range items {
		if it.Type == "REMINDER_OVERDUE" {
			return true
		}
	}
	return false
}

// adminAuditKey groups escalated items by (admin, audit) — one digest email
// per key, since the digest's subject names a single audit.
type adminAuditKey struct {
	adminID int
	auditID int
}

// RunOnce runs the sweep synchronously and returns its error, if any. Two
// callers share it: the scheduler's daily tick (internal/scheduler) and the
// manual-trigger endpoint (POST /api/v1/audits/reminders/run), which lets
// QA/ops fire a sweep without waiting for the fixed daily time.
func (j *ReminderJob) RunOnce(ctx context.Context) error {
	return j.runOnce(ctx)
}

// reminderTier returns "REMINDER_DUE_10", "REMINDER_DUE_5",
// "REMINDER_DUE_TODAY", "REMINDER_OVERDUE", or "" (no tier applies today)
// for dueDate ("YYYY-MM-DD") relative to today (UTC, date-only comparison).
func reminderTier(dueDate string, today time.Time) string {
	due, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return ""
	}
	daysUntil := int(due.Sub(dateOnly(today)).Hours() / 24)
	switch {
	case daysUntil < 0:
		return "REMINDER_OVERDUE"
	case daysUntil == 0:
		return "REMINDER_DUE_TODAY"
	case daysUntil == 5:
		return "REMINDER_DUE_5"
	case daysUntil == 10:
		return "REMINDER_DUE_10"
	default:
		return ""
	}
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// releaseCtx detaches from ctx's deadline/cancellation (but keeps its values)
// and applies releaseTimeout instead, so a release triggered by the sweep's
// own ctx expiring isn't doomed to fail for the same reason.
func releaseCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
}

// tierLabel renders a reminderTier value for the email body.
func tierLabel(tier string) string {
	switch tier {
	case "REMINDER_DUE_10":
		return "Due in 10 days"
	case "REMINDER_DUE_5":
		return "Due in 5 days"
	case "REMINDER_DUE_TODAY":
		return "Due today"
	case "REMINDER_OVERDUE":
		return "Overdue"
	default:
		return ""
	}
}

func (j *ReminderJob) runOnce(parent context.Context) (runErr error) {
	if !j.running.CompareAndSwap(false, true) {
		return errors.New("reminder job: a sweep is already running")
	}
	defer j.running.Store(false)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("reminder job: recovered from panic", "panic", r, "stack", string(debug.Stack()))
			runErr = fmt.Errorf("reminder job panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	audits, err := j.audits.List(ctx)
	if err != nil {
		slog.Error("reminder job: list audits", "err", err)
		return fmt.Errorf("list audits: %w", err)
	}
	// ACTIVE and ARCHIVED audits both stay in scope; only COMPLETED/REMOVED
	// are excluded — an archived audit can still be ongoing.
	activeAuditIDs := make(map[int]bool, len(audits))
	// Names come from this same already-fetched list, so the overdue admin
	// alert can head each email with its audit without a lookup per email.
	auditNames := make(map[int]string, len(audits))
	for _, a := range audits {
		if a.Status != "COMPLETED" && a.Status != "REMOVED" {
			activeAuditIDs[a.ID] = true
		}
		auditNames[a.ID] = a.Name
	}

	// Resolved once per sweep, not per control. A failure here is logged and
	// treated as "no admins": the owner reminders below are the primary
	// delivery and must not be lost to a failed grant lookup.
	var adminIDs []int
	if j.admins != nil && j.notifyAdmin != nil {
		adminIDs, err = j.admins(ctx)
		if err != nil {
			slog.Warn("reminder job: failed to resolve admin recipients, skipping overdue escalation this run", "err", err)
			adminIDs = nil
		}
	}

	controls, err := j.controls.ListAllForReminders(ctx)
	if err != nil {
		slog.Error("reminder job: list controls", "err", err)
		return fmt.Errorf("list controls: %w", err)
	}

	today := time.Now().UTC()
	todayStr := today.Format("2006-01-02")
	byOwner := map[int][]model.ReminderItem{}
	byAdmin := map[adminAuditKey][]model.ReminderItem{}
	queued, skippedDup, skippedErr := 0, 0, 0

	// release gives one claim back so a later run retries the item. If the
	// release itself fails, the claim IS stuck: logged loud (Error, not Warn)
	// since that's the one failure mode this mechanism doesn't self-heal from.
	// why names the path that triggered it, for the log line.
	release := func(notificationID int64, recipientID int, notifType, why string) {
		rctx, cancel := releaseCtx(ctx)
		relErr := j.claim.ReleaseClaim(rctx, notificationID)
		cancel()
		if relErr != nil {
			slog.Error("reminder job: failed to release claim — item will NOT retry until this is fixed",
				"reason", why, "notificationId", notificationID, "recipientId", recipientID, "type", notifType, "err", relErr)
		}
	}
	// Releases every still-pending claim before a panic reaches the top-level
	// recover, so nothing stays claimed forever with nothing sent. Whatever
	// remains in byOwner/byAdmin here is exactly the unresolved set.
	defer func() {
		if r := recover(); r != nil {
			for ownerID, items := range byOwner {
				for _, it := range items {
					release(it.NotificationID, ownerID, it.Type, "panic")
				}
			}
			for key, items := range byAdmin {
				for _, it := range items {
					release(it.NotificationID, key.adminID, it.Type, "panic")
				}
			}
			panic(r)
		}
	}()

	queue := func(ownerID int, item model.ReminderItem, controlID, populationID *int) {
		claimed, notificationID, err := j.claim.Claim(ctx, ownerID, item.AuditID, item.Type, controlID, populationID, &item.DedupSnapshot)
		if err != nil {
			// Fail CLOSED: a claim error means we can't tell if we hold it, so
			// sending anyway risks a duplicate. Skipping costs one day's delay.
			slog.Warn("reminder job: claim failed, skipping this run (fail closed)", "ownerId", ownerID, "type", item.Type, "err", err)
			skippedErr++
			return
		}
		if !claimed {
			skippedDup++
			return
		}
		item.NotificationID = notificationID
		byOwner[ownerID] = append(byOwner[ownerID], item)
		queued++
	}

	// queueAdmins escalates one overdue item to every admin (grouped into
	// that admin's digest per audit), claiming per admin per item. An admin
	// who owns the item is skipped — they already hear about it in their own digest.
	queueAdmins := func(item model.ReminderItem, controlID, populationID *int) {
		for _, adminID := range adminIDs {
			if adminID == item.OwnerUserID {
				continue
			}
			claimed, notificationID, err := j.claim.Claim(ctx, adminID, item.AuditID, item.Type, controlID, populationID, &item.DedupSnapshot)
			if err != nil {
				// Fail closed, exactly as queue does.
				slog.Warn("reminder job: admin claim failed, skipping this run (fail closed)", "adminId", adminID, "type", item.Type, "err", err)
				skippedErr++
				continue
			}
			if !claimed {
				skippedDup++
				continue
			}
			item.NotificationID = notificationID
			key := adminAuditKey{adminID: adminID, auditID: item.AuditID}
			byAdmin[key] = append(byAdmin[key], item)
			queued++
		}
	}

	for _, c := range controls {
		if c == nil || !activeAuditIDs[c.AuditID] {
			continue
		}
		if c.OwnerID != nil && c.Status != "COMPLETE" && c.DueDate != nil {
			if tier := reminderTier(*c.DueDate, today); tier != "" {
				dedupSnapshot := *c.DueDate
				if tier == "REMINDER_OVERDUE" {
					dedupSnapshot = todayStr // re-fires daily — see model.ReminderItem.DedupSnapshot
				}
				item := model.ReminderItem{
					AuditID:         c.AuditID,
					ControlID:       &c.ID,
					Type:            tier,
					ControlNumber:   c.ControlNumber,
					Description:     c.Description,
					DueDate:         *c.DueDate,
					Tier:            tierLabel(tier),
					RequirementType: "Evidence Requirement",
					DedupSnapshot:   dedupSnapshot,
					AuditName:       auditNames[c.AuditID],
					LinkControlID:   c.ID,
					OwnerUserID:     *c.OwnerID,
				}
				queue(*c.OwnerID, item, &c.ID, nil)
				if tier == "REMINDER_OVERDUE" {
					queueAdmins(item, &c.ID, nil)
				}
			}
		}
		if c.PopulationOwnerID != nil && (c.PopulationStatus == nil || *c.PopulationStatus != "APPROVED") && c.PopulationDueDate != nil {
			if tier := reminderTier(*c.PopulationDueDate, today); tier != "" {
				dedupSnapshot := *c.PopulationDueDate
				if tier == "REMINDER_OVERDUE" {
					dedupSnapshot = todayStr
				}
				item := model.ReminderItem{
					AuditID:         c.AuditID,
					PopulationID:    c.PopulationID,
					Type:            tier,
					ControlNumber:   c.ControlNumber,
					Description:     c.Description,
					DueDate:         *c.PopulationDueDate,
					Tier:            tierLabel(tier),
					RequirementType: "Population Requirement",
					DedupSnapshot:   dedupSnapshot,
					AuditName:       auditNames[c.AuditID],
					LinkControlID:   c.ID,
					OwnerUserID:     *c.PopulationOwnerID,
				}
				queue(*c.PopulationOwnerID, item, nil, c.PopulationID)
				if tier == "REMINDER_OVERDUE" {
					queueAdmins(item, nil, c.PopulationID)
				}
			}
		}
	}

	// Lead recipients, resolved once per sweep and only for owners who
	// actually have overdue items. Stays nil when the escalation is disabled,
	// and a nil map reads as "no lead" for every owner below.
	var ownerLeads map[int]string
	if j.leads != nil && j.notifyLead != nil {
		overdueOwners := make([]int, 0, len(byOwner))
		for ownerID, items := range byOwner {
			if hasOverdue(items) {
				overdueOwners = append(overdueOwners, ownerID)
			}
		}
		if len(overdueOwners) > 0 {
			ownerLeads = j.leads(ctx, overdueOwners)
		}
	}

	sent, notifyFailed := 0, 0
	leadSent, leadFailed := 0, 0
	for ownerID, items := range byOwner {
		if err := j.notify(ctx, ownerID, items); err != nil {
			slog.Warn("reminder job: notification failed", "ownerId", ownerID, "items", len(items), "err", err)
			// The digest failed, so every item it covered must give up its
			// claim — otherwise it stays claimed forever with nothing sent.
			for _, it := range items {
				release(it.NotificationID, ownerID, it.Type, "failed send")
			}
			notifyFailed++
			delete(byOwner, ownerID) // resolved (failed+released) — the panic-recovery defer above must not also release it
			continue
		}
		sent++
		delete(byOwner, ownerID) // resolved (sent) — must never be released, even if a later owner's notify panics
		// Strictly after the delete above: the digest is out and its claims are
		// spent, so neither a failure nor a panic here may release them —
		// doing so would re-send the owner's own digest tomorrow for an item
		// they were already told about.
		if leadEmail := ownerLeads[ownerID]; leadEmail != "" {
			if overdue := overdueOnly(items); len(overdue) > 0 {
				if err := j.notifyLead(ctx, ownerID, leadEmail, overdue); err != nil {
					slog.Warn("reminder job: lead escalation failed", "ownerId", ownerID, "items", len(overdue), "err", err)
					leadFailed++
				} else {
					leadSent++
				}
			}
		}
	}

	// Resolved once per sweep, deduped across every escalated item, so an
	// owner with several overdue items (or several admins) is looked up once
	// instead of once per email.
	if len(byAdmin) > 0 && j.resolveOwnerNames != nil {
		ownerIDSet := map[int]bool{}
		for _, items := range byAdmin {
			for _, it := range items {
				ownerIDSet[it.OwnerUserID] = true
			}
		}
		ownerIDs := make([]int, 0, len(ownerIDSet))
		for id := range ownerIDSet {
			ownerIDs = append(ownerIDs, id)
		}
		ownerNames := j.resolveOwnerNames(ctx, ownerIDs)
		for _, items := range byAdmin {
			for i := range items {
				items[i].OwnerName = ownerNames[items[i].OwnerUserID]
			}
		}
	}

	// Overdue escalations: one digest email per (admin, audit). Each stands
	// alone, so unlike the owner loop above a failure releases only its own
	// group's claims and every other digest still goes out.
	adminSent, adminFailed := 0, 0
	for key, items := range byAdmin {
		if err := j.notifyAdmin(ctx, key.adminID, items); err != nil {
			slog.Warn("reminder job: overdue admin digest failed",
				"adminId", key.adminID, "auditId", key.auditID, "items", len(items), "err", err)
			for _, it := range items {
				release(it.NotificationID, key.adminID, it.Type, "failed admin send")
			}
			adminFailed++
			delete(byAdmin, key) // resolved (failed+released) — the panic-recovery defer above must not also release it
			continue
		}
		adminSent++
		delete(byAdmin, key) // resolved (sent) — must never be released, even if a later digest's send panics
	}

	slog.Info("reminder job: run complete",
		"owners", sent, "notifyFailed", notifyFailed,
		"adminDigestsSent", adminSent, "adminDigestsFailed", adminFailed,
		"leadDigestsSent", leadSent, "leadDigestsFailed", leadFailed,
		"itemsQueued", queued, "itemsSkippedDup", skippedDup, "itemsSkippedErr", skippedErr)
	return nil
}

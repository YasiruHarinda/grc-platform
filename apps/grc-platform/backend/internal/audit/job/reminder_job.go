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

// auditLister and controlLister are the only two read capabilities this job
// needs, kept as narrow local interfaces (not the full
// repository.AuditRepository/ControlRepository) so this package doesn't
// import internal/audit/repository or internal/audit/service — which would
// import back into internal/audit/handler (via the notify function field
// below) and cycle. Same reasoning as internal/risk/job.
type auditLister interface {
	List(ctx context.Context) ([]*model.Audit, error)
}

type controlLister interface {
	ListAllForReminders(ctx context.Context) ([]*model.AuditControl, error)
}

// claimer is the reminder job's own de-dup gate — structurally satisfied by
// auditservice.NotificationService.Claim/ReleaseClaim without this package
// importing that package. The insert Claim triggers is the atomic de-dup
// decision (see docs/new/Reminder-Notification-Atomic-Claim-Design.md);
// ReleaseClaim undoes it when the owner's digest then fails to send, so the
// item is retried on a future run instead of staying claimed forever.
type claimer interface {
	Claim(ctx context.Context, recipientID, auditID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (claimed bool, notificationID int64, err error)
	ReleaseClaim(ctx context.Context, notificationID int64) error
}

const (
	// reminderHourUTC is when the daily digest fires. UTC, matching how the
	// database's own DATETIME columns are pinned (see internal/db).
	reminderHourUTC = 8
	// runTimeout bounds a single sweep.
	runTimeout = 30 * time.Minute
)

// ReminderJob sweeps every control/population item daily and emails each
// owner one combined digest of everything due in 10 days, due in 5 days, or
// overdue.
type ReminderJob struct {
	audits   auditLister
	controls controlLister
	claim    claimer
	// notify delivers one owner's full daily digest synchronously. A plain
	// function (not a handler dependency) so this package doesn't import
	// internal/audit/handler (which imports service → would cycle) — wired to
	// handler.Deps.SendReminderDigestSync at startup, exactly as
	// internal/risk/job.EscalationJob is wired to NotifyEscalationSync. Unlike
	// every other notify path, this one does NOT log audit_notification rows
	// on success — the claim already did, before the send was even attempted
	// (see claimer's doc comment).
	notify func(ctx context.Context, ownerUserID int, items []model.ReminderItem) error
	// running serializes runOnce against itself: Start's daily ticker and the
	// manual-trigger endpoint (handler.reminderJobHandler.run) both end up
	// calling runOnce on this same instance. This guards a same-process
	// overlap cheaply; claimer's DB-level unique constraint is what makes the
	// de-dup correct across processes/replicas too — see
	// docs/new/Reminder-Notification-Atomic-Claim-Design.md.
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

// Start waits until the next occurrence of reminderHourUTC, runs the sweep,
// then repeats daily, until ctx is cancelled. Intended to be launched in its
// own goroutine from main.
//
// Deliberately not a ticker fired immediately at boot (unlike
// internal/risk/job.EscalationJob): a reminder digest wants a predictable,
// fixed send time regardless of when the server last restarted, not "24h
// since whenever the process happened to start."
func (j *ReminderJob) Start(ctx context.Context) {
	for {
		timer := time.NewTimer(durationUntilNext(reminderHourUTC, time.Now().UTC()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			_ = j.runOnce(ctx)
		}
	}
}

// durationUntilNext returns the wait until the next occurrence of hour:00
// UTC — today's if it hasn't passed yet, tomorrow's otherwise. A pure
// function of (hour, now) so it's unit-testable without mocking time.Now.
func durationUntilNext(hour int, now time.Time) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}

// RunOnce runs the sweep synchronously and returns its error, if any — the
// manual-trigger endpoint's entry point (POST /api/v1/audits/reminders/run),
// for QA/ops convenience without waiting for the fixed daily time.
func (j *ReminderJob) RunOnce(ctx context.Context) error {
	return j.runOnce(ctx)
}

// reminderTier returns "REMINDER_DUE_10", "REMINDER_DUE_5",
// "REMINDER_OVERDUE", or "" (no tier applies today) for dueDate
// ("YYYY-MM-DD") relative to today (UTC, date-only comparison).
func reminderTier(dueDate string, today time.Time) string {
	due, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return ""
	}
	daysUntil := int(due.Sub(dateOnly(today)).Hours() / 24)
	switch {
	case daysUntil < 0:
		return "REMINDER_OVERDUE"
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

// tierLabel renders a reminderTier value for the email body.
func tierLabel(tier string) string {
	switch tier {
	case "REMINDER_DUE_10":
		return "Due in 10 days"
	case "REMINDER_DUE_5":
		return "Due in 5 days"
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
	for _, a := range audits {
		if a.Status != "COMPLETED" && a.Status != "REMOVED" {
			activeAuditIDs[a.ID] = true
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
	queued, skippedDup, skippedErr := 0, 0, 0

	// Release every still-pending claim before letting a panic reach the
	// top-level recover above — otherwise those items would stay claimed
	// forever with nothing sent, the same stuck-claim outcome the ordinary
	// release-on-failed-send path (below) exists to avoid. Registered here
	// (after byOwner exists, so it can reference it) rather than with the
	// top-level recover: defers run LIFO, so this one fires first and can
	// still re-panic for the top-level recover to log and convert to an
	// error, same as any other panic. The send loop below deletes each
	// owner's entry from byOwner as it's resolved (success or ordinary
	// failure), so whatever remains here is exactly the unresolved set.
	defer func() {
		if r := recover(); r != nil {
			for ownerID, items := range byOwner {
				for _, it := range items {
					if relErr := j.claim.ReleaseClaim(ctx, it.NotificationID); relErr != nil {
						slog.Error("reminder job: failed to release claim after a panic — item will NOT retry until this is fixed",
							"notificationId", it.NotificationID, "ownerId", ownerID, "type", it.Type, "err", relErr)
					}
				}
			}
			panic(r)
		}
	}()

	queue := func(ownerID int, item model.ReminderItem, controlID, populationID *int) {
		claimed, notificationID, err := j.claim.Claim(ctx, ownerID, item.AuditID, item.Type, controlID, populationID, &item.DedupSnapshot)
		if err != nil {
			// Fail CLOSED, unlike the old Exists-based check's fail-open
			// ("sending anyway"): once a claim call itself errors, this run
			// can't tell whether it actually holds the claim or not, so
			// sending anyway risks exactly the unguarded duplicate this
			// mechanism exists to prevent. Skipping costs one day's delay —
			// tomorrow's run retries it.
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
				queue(*c.OwnerID, model.ReminderItem{
					AuditID:         c.AuditID,
					ControlID:       &c.ID,
					Type:            tier,
					ControlNumber:   c.ControlNumber,
					Description:     c.Description,
					DueDate:         *c.DueDate,
					Tier:            tierLabel(tier),
					RequirementType: "Evidence Requirement",
					DedupSnapshot:   dedupSnapshot,
				}, &c.ID, nil)
			}
		}
		if c.PopulationOwnerID != nil && (c.PopulationStatus == nil || *c.PopulationStatus != "APPROVED") && c.PopulationDueDate != nil {
			if tier := reminderTier(*c.PopulationDueDate, today); tier != "" {
				dedupSnapshot := *c.PopulationDueDate
				if tier == "REMINDER_OVERDUE" {
					dedupSnapshot = todayStr
				}
				queue(*c.PopulationOwnerID, model.ReminderItem{
					AuditID:         c.AuditID,
					PopulationID:    c.PopulationID,
					Type:            tier,
					ControlNumber:   c.ControlNumber,
					Description:     c.Description,
					DueDate:         *c.PopulationDueDate,
					Tier:            tierLabel(tier),
					RequirementType: "Population Requirement",
					DedupSnapshot:   dedupSnapshot,
				}, nil, c.PopulationID)
			}
		}
	}

	sent, notifyFailed := 0, 0
	for ownerID, items := range byOwner {
		if err := j.notify(ctx, ownerID, items); err != nil {
			slog.Warn("reminder job: notification failed", "ownerId", ownerID, "items", len(items), "err", err)
			// The digest didn't go out, so every item it would have covered
			// must give up its claim — otherwise they'd stay claimed forever
			// with nothing ever sent, silently starving that owner of
			// reminders. If the release itself fails, the claim IS stuck: log
			// loud (Error, not Warn) since this is the one failure mode this
			// mechanism doesn't self-heal from — see
			// docs/new/Reminder-Notification-Atomic-Claim-Design.md §4.
			for _, it := range items {
				if relErr := j.claim.ReleaseClaim(ctx, it.NotificationID); relErr != nil {
					slog.Error("reminder job: failed to release claim after failed send — item will NOT retry until this is fixed",
						"notificationId", it.NotificationID, "ownerId", ownerID, "type", it.Type, "err", relErr)
				}
			}
			notifyFailed++
			delete(byOwner, ownerID) // resolved (failed+released) — the panic-recovery defer above must not also release it
			continue
		}
		sent++
		delete(byOwner, ownerID) // resolved (sent) — must never be released, even if a later owner's notify panics
	}
	slog.Info("reminder job: run complete",
		"owners", sent, "notifyFailed", notifyFailed, "itemsQueued", queued, "itemsSkippedDup", skippedDup, "itemsSkippedErr", skippedErr)
	return nil
}

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

// Package directorysync marks users the identity directory reports as disabled,
// after telling the admins who can reassign the work they leave behind.
package directorysync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync/atomic"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

const (
	// A run does a SCIM lookup per external user and one email per admin. This
	// only stops a wedged run from blocking the next one.
	runTimeout = 30 * time.Minute

	// StatusDisabled is the user.status value the sync writes.
	StatusDisabled = user.StatusDisabled

	// The two statuses a user can be moved out of. Skipping anyone already
	// Disabled is also what stops a departure being emailed twice.
	statusActive   = user.StatusActive
	statusInactive = user.StatusInactive

	// UserTypeExternal identifies a user in the external Asgardeo org, resolved
	// one uuid at a time since that org has no bulk snapshot.
	UserTypeExternal = "EXTERNAL"

	// How old the snapshot may be and still be trusted. The refresh runs daily
	// and retries every 15 minutes, so older means many retries have failed.
	snapshotMaxAge = 26 * time.Hour

	// maxDeparturesPerRun caps how many users one run will disable. Real
	// departures trickle in a few at a time; a run that finds more than this
	// is almost certainly an upstream fault — a bad directory snapshot
	// reporting everyone disabled — and is aborted whole, writing no status
	// and sending no digest, for a human to check.
	maxDeparturesPerRun = 20
)

// User is one platform user the sync checks.
type User struct {
	ID       int
	UUID     string
	UserType string
	Status   string
}

// Assignment is one piece of live work a departed user is still named on.
// Forward-looking attachments only, never a record of a past act.
type Assignment struct {
	// The thing to reassign: a control number and description, or a risk code
	// and title.
	Item       string
	ItemDetail string
	// Parent is the audit or the source register the item sits under.
	Parent string
	// Role is what the departed person held on this item.
	Role string
	// Standing is the one column whose meaning belongs to the hub rather than
	// this package — a due date in audit, a workflow status in risk. The job
	// only carries it; the hub that set it is the only thing that reads it.
	Standing  string
	DetailURL string
	// Blocking marks a row whose parent is stuck until it is reassigned — a risk
	// sitting at management approval whose approver has left.
	Blocking bool
}

// Departure is one newly disabled person and the live work they leave behind.
type Departure struct {
	UserID int
	// Name falls back to email then uuid; a digest naming nobody is useless.
	Name        string
	Assignments []Assignment
}

// DirectoryState is what the directory says about an account, plus whether its
// snapshot can be trusted this run. Satisfied by *directory.Service.
type DirectoryState interface {
	// found=false means absent from the snapshot, which is a different fact from
	// present-but-carrying-no-attribute and is counted separately.
	SnapshotState(uuid string) (state scim.AccountState, found bool)
	SnapshotStatus() (size int, refreshedAt time.Time)
	// An error here is no information, exactly like a not-found.
	ExternalState(ctx context.Context, uuid string) (scim.AccountState, error)
}

// Hub is one module's half of a run: what work it can find for the departed,
// who it tells about it, and how.
type Hub struct {
	// Name identifies the hub in log lines ("audit", "risk").
	Name string
	// Assignments returns each user's ongoing assignments, keyed by user id. A
	// user with none may be omitted or map to an empty slice.
	Assignments func(ctx context.Context, userIDs []int) (map[int][]Assignment, error)
	// Recipients resolves the admins who can reassign this hub's work.
	Recipients func(ctx context.Context) ([]int, error)
	// Notify delivers one recipient's digest synchronously. A recipient it cannot
	// resolve must be an error, not a skip — the status write depends on this.
	Notify func(ctx context.Context, recipientUserID int, departures []Departure) error
}

// Deps is everything the job needs, each piece narrow enough to fake.
type Deps struct {
	Users func(ctx context.Context) ([]User, error)
	// Directory resolves disabled state by both paths.
	Directory DirectoryState
	// ResolveName never fails: an unresolvable person degrades to their uuid.
	ResolveName func(ctx context.Context, uuid, userType string) string
	// Disable writes the status and records it in the admin activity log under
	// the sync's reserved actor. Returns false when the row was already gone,
	// so nothing was written, logged or counted.
	Disable func(ctx context.Context, u User, name string) (bool, error)
	Hubs    []Hub
	// EmailEnabled mirrors the platform-wide notification switch. Off means
	// statuses are still written and no send is attempted.
	EmailEnabled bool
	// ProtectedAdminIDs returns the user ids that currently hold MANAGE_USERS.
	// One run is never allowed to disable every one of them at once — see
	// spareAdmins. nil disables that guard.
	ProtectedAdminIDs func(ctx context.Context) ([]int, error)
}

// Job is the Directory Status Sync, shared by the scheduler and the manual
// trigger.
type Job struct {
	deps Deps
	// running serializes RunOnce against itself, as the reminder job does.
	running atomic.Bool
}

// New constructs a Job.
func New(deps Deps) *Job { return &Job{deps: deps} }

// RunOnce resolves disabled state, gathers ongoing assignments, sends the
// digests, then writes statuses. A second concurrent call is refused.
func (j *Job) RunOnce(ctx context.Context) error {
	if !j.running.CompareAndSwap(false, true) {
		return errors.New("directory status sync: a run is already in progress")
	}
	defer j.running.Store(false)
	return j.runOnce(ctx, false)
}

// Trigger claims the run slot and runs detached, reporting false if a run is
// already in progress. The manual trigger replies before the run finishes, so
// it must claim the slot to answer 409 rather than discovering the clash inside
// a goroutine nobody is watching — and the slot is this one, since the
// scheduled sweep and the manual trigger share a single Job.
//
// overrideLimit lets an operator push a genuine batch (a contractor engagement
// ending, a team offboarded together) past maxDeparturesPerRun for this one
// run. Scheduled runs never set it, so the unattended path stays capped.
func (j *Job) Trigger(overrideLimit bool) bool {
	if !j.running.CompareAndSwap(false, true) {
		return false
	}
	go func() { // #nosec G118 -- deliberately on a background context: the caller's would be cancelled the instant it answers 202, well before an up-to-30min run finishes
		defer j.running.Store(false)
		if err := j.runOnce(context.Background(), overrideLimit); err != nil {
			slog.Error("directory status sync: manual trigger failed", "err", err)
		}
	}()
	return true
}

// counts is the one log line every run ends with. noAttribute matching the whole
// internal user base means the attributes are not coming back and this is inert
// — which only holds while it counts the snapshot alone, so the external half's
// single unresolved answer is kept apart in externalUnresolved.
type counts struct {
	checked      int
	disabled     int
	withOpenWork int
	emailsSent   int
	errs         int
	noUUID       int
	absent       int
	noAttribute  int
	// externalUnresolved is the external org's one merged answer: not-found and
	// present-but-no-attribute are indistinguishable without a snapshot.
	externalUnresolved int
	internalSkipped    int
}

func (j *Job) runOnce(parent context.Context, overrideLimit bool) (runErr error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("directory status sync: recovered from panic", "panic", r, "stack", string(debug.Stack()))
			runErr = fmt.Errorf("directory status sync panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	users, err := j.deps.Users(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "directory status sync: list users", "err", err)
		return fmt.Errorf("list users: %w", err)
	}

	var c counts
	departed := j.resolveDeparted(ctx, users, &c)
	if len(departed) == 0 {
		// A run that found nobody logs like one that found somebody, so silence
		// is never ambiguous.
		logSummary(ctx, c)
		return nil
	}
	if !overrideLimit && len(departed) > maxDeparturesPerRun {
		// Treat an implausible count as an upstream fault, not a fact: no
		// status is written and no digest is sent this run. A genuine batch
		// goes through the manual trigger with its override set.
		slog.ErrorContext(ctx, "directory status sync: too many users reported disabled, aborting run",
			"departed", len(departed), "limit", maxDeparturesPerRun)
		logSummary(ctx, c)
		return fmt.Errorf("directory status sync: %d users reported disabled exceeds the %d-per-run safety limit "+
			"(retry the manual trigger with override=true if this batch is genuine)",
			len(departed), maxDeparturesPerRun)
	}
	if overrideLimit && len(departed) > maxDeparturesPerRun {
		slog.WarnContext(ctx, "directory status sync: per-run safety limit overridden by manual trigger",
			"departed", len(departed), "limit", maxDeparturesPerRun)
	}

	departed = j.spareAdmins(ctx, departed, users, &c)
	if len(departed) == 0 {
		logSummary(ctx, c)
		return nil
	}

	names := make(map[int]string, len(departed))
	ids := make([]int, 0, len(departed))
	for _, u := range departed {
		names[u.ID] = j.deps.ResolveName(ctx, u.UUID, u.UserType)
		ids = append(ids, u.ID)
	}

	// All-or-nothing per run: without knowing what a hub holds, a status write
	// would disable someone against a notice that never named their work.
	hubWork := make([]map[int][]Assignment, len(j.deps.Hubs))
	for i, hub := range j.deps.Hubs {
		work, err := hub.Assignments(ctx, ids)
		if err != nil {
			c.errs++
			slog.ErrorContext(ctx, "directory status sync: gather ongoing assignments",
				"hub", hub.Name, "err", err)
			logSummary(ctx, c)
			return fmt.Errorf("gather %s assignments: %w", hub.Name, err)
		}
		hubWork[i] = work
	}

	withWork := map[int]bool{}
	for _, work := range hubWork {
		for id, items := range work {
			if len(items) > 0 {
				withWork[id] = true
			}
		}
	}
	c.withOpenWork = len(withWork)

	// delivered[i] gates writing the status of anyone hub i's digest was about.
	delivered := make([]bool, len(j.deps.Hubs))
	for i, hub := range j.deps.Hubs {
		delivered[i] = j.sendDigest(ctx, hub, groupByPerson(departed, names, hubWork[i]), &c)
	}

	for _, u := range departed {
		if blocked, hub := notifyPending(u.ID, hubWork, delivered, j.deps.Hubs); blocked {
			slog.WarnContext(ctx, "directory status sync: digest not delivered, leaving status unwritten for retry",
				"userId", u.ID, "hub", hub)
			continue
		}
		disabled, err := j.deps.Disable(ctx, u, names[u.ID])
		if err != nil {
			c.errs++
			slog.ErrorContext(ctx, "directory status sync: write status", "userId", u.ID, "err", err)
			continue
		}
		if !disabled {
			// Row deleted between this run's listing and the write: nothing to
			// disable, and nothing was logged.
			slog.WarnContext(ctx, "directory status sync: user row gone before status write, skipping", "userId", u.ID)
			continue
		}
		c.disabled++
	}

	logSummary(ctx, c)
	return nil
}

// spareAdmins refuses to let one run disable every remaining MANAGE_USERS
// holder. The directory reporting all of them disabled at once is bad data, not
// a fact: acting on it would leave nobody able to move any user back to Active
// (Disabled is system-owned and cannot be cleared by hand without the
// privilege). Those departures are dropped from the run and logged loudly; a
// human moves them out of Active if the batch was genuine.
func (j *Job) spareAdmins(ctx context.Context, departed, allUsers []User, c *counts) []User {
	if j.deps.ProtectedAdminIDs == nil {
		return departed
	}
	holderIDs, err := j.deps.ProtectedAdminIDs(ctx)
	if err != nil {
		c.errs++
		slog.ErrorContext(ctx, "directory status sync: could not resolve MANAGE_USERS holders; last-admin guard inactive this run", "err", err)
		return departed
	}
	status := make(map[int]string, len(allUsers))
	for _, u := range allUsers {
		status[u.ID] = u.Status
	}
	holders := make(map[int]bool, len(holderIDs))
	for _, id := range holderIDs {
		if s := status[id]; s == statusActive || s == statusInactive {
			holders[id] = true
		}
	}
	if len(holders) == 0 {
		return departed
	}
	inRun := 0
	for _, u := range departed {
		if holders[u.ID] {
			inRun++
		}
	}
	if inRun < len(holders) {
		// At least one holder is outside this run — the platform keeps an admin.
		return departed
	}
	kept := make([]User, 0, len(departed))
	skipped := 0
	for _, u := range departed {
		if holders[u.ID] {
			skipped++
			continue
		}
		kept = append(kept, u)
	}
	slog.ErrorContext(ctx, "directory status sync: every active MANAGE_USERS holder was reported disabled in one run — leaving them Active, nothing written or emailed for them",
		"skippedAdmins", skipped)
	c.internalSkipped += skipped
	return kept
}

// resolveDeparted returns the users the directory now reports as disabled,
// counting every reason the rest were left alone.
func (j *Job) resolveDeparted(ctx context.Context, users []User, c *counts) []User {
	size, refreshedAt := j.deps.Directory.SnapshotStatus()
	snapshotUsable := size > 0 && !refreshedAt.IsZero() && time.Since(refreshedAt) <= snapshotMaxAge
	if !snapshotUsable {
		// The external half reads no snapshot and runs either way: a cold start
		// is not a reason to stop noticing that auditors have left.
		slog.WarnContext(ctx, "directory status sync: snapshot empty or stale, skipping the internal half of this run",
			"snapshotSize", size, "refreshedAt", refreshedAt, "maxAge", snapshotMaxAge)
	}

	departed := make([]User, 0, 8)
	for _, u := range users {
		if u.Status != statusActive && u.Status != statusInactive {
			continue
		}
		if u.UUID == "" {
			c.noUUID++
			continue
		}
		if u.UserType == UserTypeExternal {
			c.checked++
			state, err := j.deps.Directory.ExternalState(ctx, u.UUID)
			if err != nil {
				// One failing lookup says nothing about that person and nothing
				// at all about the others.
				c.errs++
				slog.WarnContext(ctx, "directory status sync: external lookup failed, leaving user unchanged",
					"userId", u.ID, "err", err)
				continue
			}
			switch state {
			case scim.AccountDisabled:
				departed = append(departed, u)
			case scim.AccountUnknown:
				// Counted apart from noAttribute: not-found and no-attribute
				// arrive as one answer here, and mixing them in would break the
				// only reading that says the attributes are not coming back.
				c.externalUnresolved++
			}
			continue
		}
		if !snapshotUsable {
			c.internalSkipped++
			continue
		}
		c.checked++
		state, found := j.deps.Directory.SnapshotState(u.UUID)
		switch {
		case !found:
			c.absent++
		case state == scim.AccountDisabled:
			departed = append(departed, u)
		case state == scim.AccountUnknown:
			c.noAttribute++
		}
	}
	return departed
}

// groupByPerson builds one hub's digest. Someone with nothing in this hub is
// omitted; ordered by name so a digest reads the same way every time.
func groupByPerson(departed []User, names map[int]string, work map[int][]Assignment) []Departure {
	out := make([]Departure, 0, len(departed))
	for _, u := range departed {
		items := work[u.ID]
		if len(items) == 0 {
			continue
		}
		out = append(out, Departure{UserID: u.ID, Name: names[u.ID], Assignments: items})
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].Name != out[k].Name {
			return out[i].Name < out[k].Name
		}
		return out[i].UserID < out[k].UserID
	})
	return out
}

// sendDigest reports whether the digest reached at least one recipient. Nothing
// to send counts as delivered: there is no notice to lose.
func (j *Job) sendDigest(ctx context.Context, hub Hub, departures []Departure, c *counts) bool {
	if len(departures) == 0 {
		return true
	}
	if !j.deps.EmailEnabled {
		slog.InfoContext(ctx, "directory status sync: notifications disabled, skipping digest",
			"hub", hub.Name, "people", len(departures))
		return true
	}

	recipients, err := hub.Recipients(ctx)
	if err != nil {
		c.errs++
		slog.ErrorContext(ctx, "directory status sync: resolve digest recipients", "hub", hub.Name, "err", err)
		return false
	}
	if len(recipients) == 0 {
		c.errs++
		slog.ErrorContext(ctx, "directory status sync: nobody can be told about these departures",
			"hub", hub.Name, "people", len(departures))
		return false
	}

	sent := 0
	for _, id := range recipients {
		if err := hub.Notify(ctx, id, departures); err != nil {
			c.errs++
			slog.WarnContext(ctx, "directory status sync: digest send failed",
				"hub", hub.Name, "recipientId", id, "err", err)
			continue
		}
		sent++
	}
	c.emailsSent += sent
	return sent > 0
}

// notifyPending reports whether userID is in a digest that reached nobody, so
// their status stays unwritten and the whole cycle retries on the next run.
func notifyPending(userID int, hubWork []map[int][]Assignment, delivered []bool, hubs []Hub) (bool, string) {
	for i, work := range hubWork {
		if len(work[userID]) > 0 && !delivered[i] {
			return true, hubs[i].Name
		}
	}
	return false, ""
}

func logSummary(ctx context.Context, c counts) {
	slog.InfoContext(ctx, "directory status sync: run complete",
		"checked", c.checked, "disabled", c.disabled, "withOpenWork", c.withOpenWork,
		"emailsSent", c.emailsSent, "errors", c.errs,
		"usersWithoutUuid", c.noUUID, "absentFromSnapshot", c.absent,
		"noDisabledAttribute", c.noAttribute, "externalUnresolved", c.externalUnresolved,
		"internalSkipped", c.internalSkipped)
}

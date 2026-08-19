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

package job

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
)

func TestDurationUntilNextTargetHour(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{
			name: "before target hour today",
			now:  time.Date(2026, 3, 5, 3, 0, 0, 0, time.UTC),
			want: 5 * time.Hour,
		},
		{
			name: "after target hour today rolls to tomorrow",
			now:  time.Date(2026, 3, 5, 14, 0, 0, 0, time.UTC),
			want: 18 * time.Hour,
		},
		{
			name: "exactly at target hour rolls to tomorrow, not zero",
			now:  time.Date(2026, 3, 5, 8, 0, 0, 0, time.UTC),
			want: 24 * time.Hour,
		},
		{
			name: "crosses a UTC day/month boundary",
			now:  time.Date(2026, 2, 28, 23, 0, 0, 0, time.UTC),
			want: 9 * time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := durationUntilNext(8, tt.now)
			if got != tt.want {
				t.Errorf("durationUntilNext(8, %v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestReminderTierBoundaries(t *testing.T) {
	today := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		dueDate string
		want    string
	}{
		{"exactly 10 days out", "2026-03-15", "REMINDER_DUE_10"},
		{"9 days out — no tier", "2026-03-14", ""},
		{"11 days out — no tier", "2026-03-16", ""},
		{"exactly 5 days out", "2026-03-10", "REMINDER_DUE_5"},
		{"due today — no tier", "2026-03-05", ""},
		{"1 day overdue", "2026-03-04", "REMINDER_OVERDUE"},
		{"far overdue", "2026-01-01", "REMINDER_OVERDUE"},
		{"unparseable date", "not-a-date", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reminderTier(tt.dueDate, today)
			if got != tt.want {
				t.Errorf("reminderTier(%q, %v) = %q, want %q", tt.dueDate, today, got, tt.want)
			}
		})
	}
}

// fakeAudits/fakeControls/fakeClaimer are minimal stand-ins for the job's
// three read/claim dependencies.
type fakeAudits struct {
	audits []*model.Audit
}

func (f *fakeAudits) List(context.Context) ([]*model.Audit, error) {
	return f.audits, nil
}

type fakeControls struct {
	controls []*model.AuditControl
}

func (f *fakeControls) ListAllForReminders(context.Context) ([]*model.AuditControl, error) {
	return f.controls, nil
}

// fakeClaimer is an in-memory stand-in for the entity's atomic claim: Claim
// on an unseen key marks it claimed and hands back a fresh notificationID;
// Claim on an already-claimed key reports claimed=false (no error) — the
// normal "lost the race" outcome. ReleaseClaim un-claims a key by its
// notificationID, so a released item can be re-claimed by a later call,
// mirroring the real DELETE-based release.
type fakeClaimer struct {
	mu      sync.Mutex
	claimed map[string]bool  // dedup key -> already claimed
	idToKey map[int64]string // notificationID -> dedup key, for release
	nextID  int64
	// released records every notificationID ReleaseClaim was called with —
	// tests assert against this to verify release-on-failed-send.
	released map[int64]bool
	// claimErr, if set, makes every Claim call fail with this error instead
	// of claiming anything — simulates the entity/network being unreachable.
	claimErr error
}

func dedupKey(recipientID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) string {
	key := fmt.Sprintf("%s|r%d", notifType, recipientID)
	if controlID != nil {
		key += fmt.Sprintf("|c%d", *controlID)
	}
	if populationID != nil {
		key += fmt.Sprintf("|p%d", *populationID)
	}
	if dueDateSnapshot != nil {
		key += "|" + *dueDateSnapshot
	}
	return key
}

func (f *fakeClaimer) Claim(_ context.Context, recipientID, auditID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (bool, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return false, 0, f.claimErr
	}
	key := dedupKey(recipientID, notifType, controlID, populationID, dueDateSnapshot)
	if f.claimed[key] {
		return false, 0, nil
	}
	if f.claimed == nil {
		f.claimed = map[string]bool{}
	}
	f.claimed[key] = true
	f.nextID++
	id := f.nextID
	if f.idToKey == nil {
		f.idToKey = map[int64]string{}
	}
	f.idToKey[id] = key
	return true, id, nil
}

func (f *fakeClaimer) ReleaseClaim(_ context.Context, notificationID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key, ok := f.idToKey[notificationID]; ok {
		delete(f.claimed, key)
	}
	if f.released == nil {
		f.released = map[int64]bool{}
	}
	f.released[notificationID] = true
	return nil
}

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func TestRunOnceSkipsCompletedAndRemovedAudits(t *testing.T) {
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 10).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{
		{ID: 1, Status: "ACTIVE"},
		{ID: 2, Status: "COMPLETED"},
		{ID: 3, Status: "REMOVED"},
		{ID: 4, Status: "ARCHIVED"}, // stays in scope per the cross-cutting rule
	}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 1, AuditID: 1, OwnerID: intPtr(101), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-1"},
		{ID: 2, AuditID: 2, OwnerID: intPtr(102), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-2"},
		{ID: 3, AuditID: 3, OwnerID: intPtr(103), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-3"},
		{ID: 4, AuditID: 4, OwnerID: intPtr(104), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-4"},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{}}
	notified := map[int]int{} // ownerID -> item count

	j := NewReminderJob(audits, controls, dedup, func(_ context.Context, ownerID int, items []model.ReminderItem) error {
		notified[ownerID] = len(items)
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}

	if _, ok := notified[101]; !ok {
		t.Error("owner 101 (ACTIVE audit) should have been notified")
	}
	if _, ok := notified[104]; !ok {
		t.Error("owner 104 (ARCHIVED audit) should have been notified")
	}
	if _, ok := notified[102]; ok {
		t.Error("owner 102 (COMPLETED audit) should NOT have been notified")
	}
	if _, ok := notified[103]; ok {
		t.Error("owner 103 (REMOVED audit) should NOT have been notified")
	}
}

func TestRunOnceSkipsNilOwner(t *testing.T) {
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 5).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 1, AuditID: 1, OwnerID: nil, Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-1"},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{}}
	called := false

	j := NewReminderJob(audits, controls, dedup, func(context.Context, int, []model.ReminderItem) error {
		called = true
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if called {
		t.Error("unassigned control must not trigger a notification")
	}
}

func TestRunOnceSkipsAlreadyLoggedReminder(t *testing.T) {
	today := time.Now().UTC()
	dueDate := today.AddDate(0, 0, 10).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 7, AuditID: 1, OwnerID: intPtr(200), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueDate), ControlNumber: "C-7"},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{
		dedupKey(200, "REMINDER_DUE_10", intPtr(7), nil, &dueDate): true,
	}}
	called := false

	j := NewReminderJob(audits, controls, dedup, func(context.Context, int, []model.ReminderItem) error {
		called = true
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if called {
		t.Error("an already-logged reminder must not be re-sent")
	}
}

func TestRunOnceBatchesControlAndPopulationIntoOneDigestPerOwner(t *testing.T) {
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 5).Format("2006-01-02")
	overdue := today.AddDate(0, 0, -3).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{
			ID: 10, AuditID: 1, ControlNumber: "C-10",
			OwnerID: intPtr(300), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon),
			PopulationID: intPtr(900), PopulationOwnerID: intPtr(300), PopulationStatus: strPtr("PENDING"), PopulationDueDate: strPtr(overdue),
		},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{}}
	var gotItems []model.ReminderItem
	calls := 0

	j := NewReminderJob(audits, controls, dedup, func(_ context.Context, ownerID int, items []model.ReminderItem) error {
		calls++
		if ownerID != 300 {
			t.Errorf("ownerID = %d, want 300", ownerID)
		}
		gotItems = items
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}

	if calls != 1 {
		t.Fatalf("notify called %d times, want exactly 1 (one combined digest per owner)", calls)
	}
	if len(gotItems) != 2 {
		t.Fatalf("digest has %d items, want 2 (one control, one population)", len(gotItems))
	}
	kinds := map[string]bool{}
	for _, it := range gotItems {
		kinds[it.RequirementType] = true
	}
	if !kinds["Evidence Requirement"] || !kinds["Population Requirement"] {
		t.Errorf("digest requirement types = %v, want both Evidence Requirement and Population Requirement", kinds)
	}
}

func TestRunOnceOverdueDedupSnapshotIsToday(t *testing.T) {
	// The overdue tier must re-fire every day, unlike the due-in-N tiers —
	// see model.ReminderItem.DedupSnapshot. Verify the dedup check is keyed
	// on today's date, not the (constant) due date, for an overdue item.
	today := time.Now().UTC()
	overdue := today.AddDate(0, 0, -10).Format("2006-01-02")
	todayStr := today.Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 5, AuditID: 1, OwnerID: intPtr(400), Status: "EVIDENCE_PENDING", DueDate: strPtr(overdue), ControlNumber: "C-5"},
	}}
	// Pretend the overdue reminder for the ORIGINAL due date was already
	// logged (e.g. a prior day's send) — this must NOT suppress today's send,
	// since the dedup key for OVERDUE is today's date, not the due date.
	dedup := &fakeClaimer{claimed: map[string]bool{
		dedupKey(400, "REMINDER_OVERDUE", intPtr(5), nil, &overdue): true,
	}}
	called := false

	j := NewReminderJob(audits, controls, dedup, func(context.Context, int, []model.ReminderItem) error {
		called = true
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if !called {
		t.Error("overdue reminder must still fire today even though the due-date-keyed row was already logged")
	}

	// Now mark TODAY's snapshot as already logged (simulating this same test
	// having already run once today) — this time it must be skipped.
	dedup.claimed[dedupKey(400, "REMINDER_OVERDUE", intPtr(5), nil, &todayStr)] = true
	called = false
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if called {
		t.Error("overdue reminder already logged for today must not be re-sent")
	}
}

func TestRunOnceOverdueItemHasCorrectFields(t *testing.T) {
	// Verifies the content of an overdue digest item, for both the control's
	// own (evidence) due date and its population's due date — not just that
	// something fires (TestRunOnceOverdueDedupSnapshotIsToday already covers
	// the re-fire timing), but that the tier label and requirement type
	// reaching the email are the right ones for each.
	today := time.Now().UTC()
	controlOverdue := today.AddDate(0, 0, -1).Format("2006-01-02")
	popOverdue := today.AddDate(0, 0, -20).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{
			ID: 11, AuditID: 1, ControlNumber: "C-11", Description: "Control desc",
			OwnerID: intPtr(500), Status: "EVIDENCE_PENDING", DueDate: strPtr(controlOverdue),
			PopulationID: intPtr(950), PopulationOwnerID: intPtr(501), PopulationStatus: strPtr("PENDING"), PopulationDueDate: strPtr(popOverdue),
		},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{}}
	digests := map[int][]model.ReminderItem{}

	j := NewReminderJob(audits, controls, dedup, func(_ context.Context, ownerID int, items []model.ReminderItem) error {
		digests[ownerID] = items
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}

	controlItems, ok := digests[500]
	if !ok || len(controlItems) != 1 {
		t.Fatalf("owner 500 digest = %v, want exactly 1 item", controlItems)
	}
	ci := controlItems[0]
	if ci.Type != "REMINDER_OVERDUE" || ci.Tier != "Overdue" {
		t.Errorf("control item Type/Tier = %q/%q, want REMINDER_OVERDUE/Overdue", ci.Type, ci.Tier)
	}
	if ci.RequirementType != "Evidence Requirement" {
		t.Errorf("control item RequirementType = %q, want Evidence Requirement", ci.RequirementType)
	}
	if ci.DueDate != controlOverdue {
		t.Errorf("control item DueDate = %q, want %q", ci.DueDate, controlOverdue)
	}

	popItems, ok := digests[501]
	if !ok || len(popItems) != 1 {
		t.Fatalf("owner 501 digest = %v, want exactly 1 item", popItems)
	}
	pi := popItems[0]
	if pi.Type != "REMINDER_OVERDUE" || pi.Tier != "Overdue" {
		t.Errorf("population item Type/Tier = %q/%q, want REMINDER_OVERDUE/Overdue", pi.Type, pi.Tier)
	}
	if pi.RequirementType != "Population Requirement" {
		t.Errorf("population item RequirementType = %q, want Population Requirement", pi.RequirementType)
	}
	if pi.DueDate != popOverdue {
		t.Errorf("population item DueDate = %q, want %q", pi.DueDate, popOverdue)
	}
}

func TestRunOnceOverdueSkipsCompleteControlAndApprovedPopulation(t *testing.T) {
	// An item overdue by date must still be excluded once it's done: a
	// COMPLETE control, or a population already APPROVED.
	today := time.Now().UTC()
	overdue := today.AddDate(0, 0, -5).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{
			ID: 20, AuditID: 1, ControlNumber: "C-20",
			OwnerID: intPtr(600), Status: "COMPLETE", DueDate: strPtr(overdue),
			PopulationID: intPtr(960), PopulationOwnerID: intPtr(601), PopulationStatus: strPtr("APPROVED"), PopulationDueDate: strPtr(overdue),
		},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{}}
	called := false

	j := NewReminderJob(audits, controls, dedup, func(context.Context, int, []model.ReminderItem) error {
		called = true
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if called {
		t.Error("a COMPLETE control and an APPROVED population must not send an overdue reminder even when past due")
	}
}

func TestRunOnceRejectsOverlappingRun(t *testing.T) {
	// Two concurrent runOnce calls on the same instance must not both proceed
	// to notify — the running guard should reject the second while the first
	// is still in flight, and let the first complete normally.
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 10).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 1, AuditID: 1, OwnerID: intPtr(1), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-1"},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{}}

	inNotify := make(chan struct{}, 1)
	releaseNotify := make(chan struct{})
	var notifyCalls atomic.Int32

	j := NewReminderJob(audits, controls, dedup, func(context.Context, int, []model.ReminderItem) error {
		notifyCalls.Add(1)
		select {
		case inNotify <- struct{}{}:
		default:
		}
		<-releaseNotify
		return nil
	})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs[0] = j.runOnce(context.Background())
	}()

	<-inNotify // wait until the first run is inside notify, mid-sweep
	errs[1] = j.runOnce(context.Background())
	close(releaseNotify)
	wg.Wait()

	if errs[1] == nil {
		t.Error("second, overlapping runOnce should have returned an error instead of running")
	}
	if errs[0] != nil {
		t.Errorf("first runOnce should have completed without error, got %v", errs[0])
	}
	if notifyCalls.Load() != 1 {
		t.Errorf("notify called %d times, want exactly 1 (the overlapping run must not send a duplicate)", notifyCalls.Load())
	}

	// The guard must release after completion, so a later, non-overlapping
	// run is still allowed to proceed. Reset the claim state first: the first
	// run's item is now legitimately, permanently claimed (it sent
	// successfully), so without this the second run would correctly skip it
	// as an already-sent duplicate — that's real dedup behavior, not what
	// this assertion is testing.
	dedup.mu.Lock()
	dedup.claimed = map[string]bool{}
	dedup.mu.Unlock()
	notifyCalls.Store(0)
	releaseNotify = make(chan struct{})
	close(releaseNotify)
	if err := j.runOnce(context.Background()); err != nil {
		t.Errorf("runOnce after the guard was released should succeed, got %v", err)
	}
	if notifyCalls.Load() != 1 {
		t.Errorf("notify called %d times after guard release, want 1", notifyCalls.Load())
	}
}

func TestRunOnceReleasesClaimOnFailedSend(t *testing.T) {
	// A claimed item whose digest send then fails must give up its claim, so
	// a later run can retry it instead of it staying claimed forever with
	// nothing ever sent — see
	// docs/new/Reminder-Notification-Atomic-Claim-Design.md §3-4.
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 10).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 1, AuditID: 1, OwnerID: intPtr(1), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-1"},
	}}
	claim := &fakeClaimer{claimed: map[string]bool{}}

	j := NewReminderJob(audits, controls, claim, func(context.Context, int, []model.ReminderItem) error {
		return fmt.Errorf("email service down")
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}

	key := dedupKey(1, "REMINDER_DUE_10", intPtr(1), nil, &dueSoon)
	if claim.claimed[key] {
		t.Error("a claim whose send failed must be released, not left claimed")
	}
	if len(claim.released) != 1 {
		t.Errorf("released = %v, want exactly one release call", claim.released)
	}

	// The item must now be re-claimable by a later run.
	claimed, _, err := claim.Claim(context.Background(), 1, 1, "REMINDER_DUE_10", intPtr(1), nil, &dueSoon)
	if err != nil || !claimed {
		t.Errorf("Claim after release = (%v, %v), want (true, nil)", claimed, err)
	}
}

func TestRunOnceSkipsItemOnClaimErrorFailClosed(t *testing.T) {
	// A Claim call that errors (e.g. the entity is unreachable) must skip the
	// item for this run rather than sending anyway — a deliberate change from
	// the old Exists-based check's fail-open behavior, since a failed claim
	// call can't tell the run whether it actually holds the claim. See
	// docs/new/Reminder-Notification-Atomic-Claim-Design.md §5.
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 5).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 1, AuditID: 1, OwnerID: intPtr(1), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-1"},
	}}
	claim := &fakeClaimer{claimed: map[string]bool{}, claimErr: fmt.Errorf("entity unreachable")}
	called := false

	j := NewReminderJob(audits, controls, claim, func(context.Context, int, []model.ReminderItem) error {
		called = true
		return nil
	})
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if called {
		t.Error("an item whose claim call errored must not be sent this run (fail closed)")
	}
}

func TestRunOnceRecoversFromPanic(t *testing.T) {
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 10).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 1, AuditID: 1, OwnerID: intPtr(1), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-1"},
	}}
	dedup := &fakeClaimer{claimed: map[string]bool{}}

	j := NewReminderJob(audits, controls, dedup, func(context.Context, int, []model.ReminderItem) error {
		panic("boom")
	})

	err := j.runOnce(context.Background())
	if err == nil {
		t.Fatal("runOnce should return an error when the job goroutine panics, not propagate the panic")
	}

	key := dedupKey(1, "REMINDER_DUE_10", intPtr(1), nil, &dueSoon)
	if dedup.claimed[key] {
		t.Error("a claim whose send panicked must be released, not left claimed — see reminder_job.go's panic-recovery defer")
	}
	if len(dedup.released) != 1 {
		t.Errorf("released = %v, want exactly one release call", dedup.released)
	}
}

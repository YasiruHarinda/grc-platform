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

// fakeAudits/fakeControls/fakeDedup are minimal stand-ins for the job's three
// read dependencies.
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

// fakeDedup reports true for any (recipient, type, controlId/populationId,
// dueDateSnapshot) tuple already in seen.
type fakeDedup struct {
	seen map[string]bool
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

func (f *fakeDedup) Exists(_ context.Context, recipientID int, notifType string, controlID, populationID *int, dueDateSnapshot *string) (bool, error) {
	return f.seen[dedupKey(recipientID, notifType, controlID, populationID, dueDateSnapshot)], nil
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
	dedup := &fakeDedup{seen: map[string]bool{}}
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
	dedup := &fakeDedup{seen: map[string]bool{}}
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
	dedup := &fakeDedup{seen: map[string]bool{
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
	dedup := &fakeDedup{seen: map[string]bool{}}
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
	dedup := &fakeDedup{seen: map[string]bool{
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
	dedup.seen[dedupKey(400, "REMINDER_OVERDUE", intPtr(5), nil, &todayStr)] = true
	called = false
	if err := j.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if called {
		t.Error("overdue reminder already logged for today must not be re-sent")
	}
}

func TestRunOnceRecoversFromPanic(t *testing.T) {
	today := time.Now().UTC()
	dueSoon := today.AddDate(0, 0, 10).Format("2006-01-02")

	audits := &fakeAudits{audits: []*model.Audit{{ID: 1, Status: "ACTIVE"}}}
	controls := &fakeControls{controls: []*model.AuditControl{
		{ID: 1, AuditID: 1, OwnerID: intPtr(1), Status: "EVIDENCE_PENDING", DueDate: strPtr(dueSoon), ControlNumber: "C-1"},
	}}
	dedup := &fakeDedup{seen: map[string]bool{}}

	j := NewReminderJob(audits, controls, dedup, func(context.Context, int, []model.ReminderItem) error {
		panic("boom")
	})

	err := j.runOnce(context.Background())
	if err == nil {
		t.Fatal("runOnce should return an error when the job goroutine panics, not propagate the panic")
	}
}

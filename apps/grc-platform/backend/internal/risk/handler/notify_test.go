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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

func ptr(s string) *string { return &s }

// listSpyEscalation records whether List was reached — enough to prove the
// LeadEscalationEmails guard in notifyEscalationLeads short-circuits before any
// work when the switch is off, and lets work through when it is on.
type listSpyEscalation struct {
	called chan int
}

func (s *listSpyEscalation) List(_ context.Context, riskID int) ([]*model.Escalation, error) {
	s.called <- riskID
	return nil, nil
}
func (s *listSpyEscalation) Escalate(context.Context, int, string) (*model.Escalation, error) {
	return nil, nil
}
func (s *listSpyEscalation) Comment(context.Context, int, int, string, string, bool) (*model.Escalation, error) {
	return nil, nil
}
func (s *listSpyEscalation) ResolveOpen(context.Context, int, string) error { return nil }

// With LEAD_ESCALATION_EMAILS_ENABLED off, notifyEscalationLeads must return
// before spawning any work — the escalation is never even loaded.
func TestNotifyEscalationLeads_SwitchOff_Noop(t *testing.T) {
	spy := &listSpyEscalation{called: make(chan int, 1)}
	d := &Deps{Escalation: spy, LeadEscalationEmails: false}

	d.notifyEscalationLeads(42, "actor-uuid")

	select {
	case <-spy.called:
		t.Fatal("Escalation.List was called despite LeadEscalationEmails=false")
	case <-time.After(100 * time.Millisecond):
	}
}

// With the switch on, notifyEscalationLeads gets as far as loading the risk's
// escalations (then no-ops here because the spy returns none).
func TestNotifyEscalationLeads_SwitchOn_LoadsEscalation(t *testing.T) {
	spy := &listSpyEscalation{called: make(chan int, 1)}
	d := &Deps{Escalation: spy, LeadEscalationEmails: true}

	d.notifyEscalationLeads(42, "actor-uuid")

	select {
	case got := <-spy.called:
		if got != 42 {
			t.Fatalf("Escalation.List riskID = %d, want 42", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Escalation.List was not called within timeout despite LeadEscalationEmails=true")
	}
}

// dedupeLeadEmails resolves an escalation's two frozen lead ids to a deduped
// address list — the assigner's and action owner's line manager are often the
// same person and must get one email, not two. Unresolvable and nil entries
// drop out.
func TestDedupeLeadEmails(t *testing.T) {
	resolve := func(byUUID map[string]string) func(context.Context, string) (string, string) {
		return func(_ context.Context, uuid string) (string, string) {
			return "", byUUID[uuid]
		}
	}

	tests := []struct {
		name string
		esc  *model.Escalation
		by   map[string]string
		want []string
	}{
		{
			name: "no leads frozen",
			esc:  &model.Escalation{},
			want: []string{},
		},
		{
			name: "two distinct leads, assigner's first",
			esc:  &model.Escalation{AssignerLeadUUID: ptr("u-assigner"), ActionOwnerLeadUUID: ptr("u-action")},
			by:   map[string]string{"u-assigner": "amir.la@example.com", "u-action": "bianca.ok@example.com"},
			want: []string{"amir.la@example.com", "bianca.ok@example.com"},
		},
		{
			name: "same person both leads collapses to one, case-insensitively",
			esc:  &model.Escalation{AssignerLeadUUID: ptr("u-1"), ActionOwnerLeadUUID: ptr("u-2")},
			by:   map[string]string{"u-1": "sol.mn@example.com", "u-2": "SOL.MN@example.com"},
			want: []string{"sol.mn@example.com"},
		},
		{
			name: "unresolvable lead is dropped",
			esc:  &model.Escalation{AssignerLeadUUID: ptr("u-known"), ActionOwnerLeadUUID: ptr("u-unknown")},
			by:   map[string]string{"u-known": "known.re@example.com"},
			want: []string{"known.re@example.com"},
		},
		{
			name: "nil action-owner lead, assigner only",
			esc:  &model.Escalation{AssignerLeadUUID: ptr("u-a")},
			by:   map[string]string{"u-a": "ann.wa@example.com"},
			want: []string{"ann.wa@example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeLeadEmails(context.Background(), tt.esc, resolve(tt.by))
			if len(got) != len(tt.want) {
				t.Fatalf("dedupeLeadEmails() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("dedupeLeadEmails()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// notifySem is shared package-wide (it bounds every notifyRiskEvent call
// site, not just this test), so this exercises the primitive directly rather
// than draining/refilling it through a real notification, which would need a
// full fake Deps for no added coverage of the thing actually being tested.
func TestNotifySemBoundsConcurrency(t *testing.T) {
	const attempts = notifyConcurrency * 4

	var current, peak int64
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			notifySem <- struct{}{}
			defer func() { <-notifySem }()

			n := atomic.AddInt64(&current, 1)
			for {
				p := atomic.LoadInt64(&peak)
				if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&current, -1)
		}()
	}
	wg.Wait()

	if peak > notifyConcurrency {
		t.Errorf("peak concurrent holders = %d, want <= %d (notifyConcurrency)", peak, notifyConcurrency)
	}
	if peak < notifyConcurrency {
		// Not a correctness failure, but if this ever fires it means the test
		// isn't actually exercising the bound (e.g. attempts too low relative
		// to the sleep), which would silently defeat its own purpose.
		t.Errorf("peak concurrent holders = %d, never reached notifyConcurrency (%d) — test isn't exercising the limit", peak, notifyConcurrency)
	}
}

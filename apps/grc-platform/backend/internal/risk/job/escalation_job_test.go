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
	"errors"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// fakeRisks models the real behaviour the paging loop depends on: an escalated
// risk drops out of the "IN_REMEDIATION and overdue" result set, so each query
// returns only what is left.
type fakeRisks struct {
	remaining []int
	queries   int
}

func (f *fakeRisks) List(context.Context, model.ListRisksFilter) (*model.RiskListPage, error) {
	f.queries++
	items := make([]*model.RiskListItem, 0, len(f.remaining))
	for _, id := range f.remaining {
		items = append(items, &model.RiskListItem{ID: id})
	}
	return &model.RiskListPage{Items: items}, nil
}

type fakeEscalator struct {
	escalated []int
	failIDs   map[int]bool
	risks     *fakeRisks
}

func (f *fakeEscalator) Escalate(_ context.Context, riskID int, _ string) (*model.Escalation, error) {
	if f.failIDs[riskID] {
		return nil, errors.New("not overdue any more")
	}
	f.escalated = append(f.escalated, riskID)
	// Mirror reality: a successfully escalated risk leaves the result set.
	left := f.risks.remaining[:0]
	for _, id := range f.risks.remaining {
		if id != riskID {
			left = append(left, id)
		}
	}
	f.risks.remaining = left
	return &model.Escalation{RiskID: riskID}, nil
}

func TestRunOnceEscalatesEveryOverdueRisk(t *testing.T) {
	risks := &fakeRisks{remaining: []int{1, 2, 3}}
	esc := &fakeEscalator{risks: risks, failIDs: map[int]bool{}}
	var notified []int

	j := NewEscalationJob(risks, esc, func(_ context.Context, id int, by string) {
		if by != escalatedBy {
			t.Errorf("notify by = %q, want %q", by, escalatedBy)
		}
		notified = append(notified, id)
	})
	j.runOnce(context.Background())

	if len(esc.escalated) != 3 {
		t.Errorf("escalated %v, want all three", esc.escalated)
	}
	// Every escalation must notify — that is the whole point of the job moving
	// out of the compliance-entity.
	if len(notified) != 3 {
		t.Errorf("notified %v, want all three", notified)
	}
	if len(risks.remaining) != 0 {
		t.Errorf("%d risks left unescalated", len(risks.remaining))
	}
}

// The loop re-queries from offset 0, so a page where nothing succeeds would
// return the same rows forever. It must give up instead of spinning.
func TestRunOnceStopsWhenNoRowCanBeEscalated(t *testing.T) {
	risks := &fakeRisks{remaining: []int{1, 2}}
	esc := &fakeEscalator{risks: risks, failIDs: map[int]bool{1: true, 2: true}}

	done := make(chan struct{})
	go func() {
		NewEscalationJob(risks, esc, nil).runOnce(context.Background())
		close(done)
	}()
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("runOnce did not terminate before the test deadline")
		}
	} else {
		<-done
	}

	if len(esc.escalated) != 0 {
		t.Errorf("escalated %v, want none", esc.escalated)
	}
	if risks.queries > 2 {
		t.Errorf("queried %d times; a fully failing page must stop after one", risks.queries)
	}
}

// A partial failure must not strand the rows that can still be escalated.
func TestRunOnceSkipsFailuresAndContinues(t *testing.T) {
	risks := &fakeRisks{remaining: []int{1, 2, 3}}
	esc := &fakeEscalator{risks: risks, failIDs: map[int]bool{2: true}}

	NewEscalationJob(risks, esc, nil).runOnce(context.Background())

	if len(esc.escalated) != 2 {
		t.Errorf("escalated %v, want 1 and 3", esc.escalated)
	}
	if len(risks.remaining) != 1 || risks.remaining[0] != 2 {
		t.Errorf("remaining = %v, want just the failing risk", risks.remaining)
	}
}

// notify is optional; a nil one must not panic the job goroutine.
func TestRunOnceToleratesNilNotify(t *testing.T) {
	risks := &fakeRisks{remaining: []int{1}}
	esc := &fakeEscalator{risks: risks, failIDs: map[int]bool{}}
	NewEscalationJob(risks, esc, nil).runOnce(context.Background())
	if len(esc.escalated) != 1 {
		t.Errorf("escalated %v, want one", esc.escalated)
	}
}

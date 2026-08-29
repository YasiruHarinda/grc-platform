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

package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Moved verbatim from internal/audit/job's reminder_job_test.go when the daily
// timer moved here.
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

// fire runs every registered sweep once.
func TestFireRunsEverySweep(t *testing.T) {
	var a, b atomic.Int32
	s := New(SweepHourUTC,
		Sweep{Name: "a", Run: func(context.Context) error { a.Add(1); return nil }},
		Sweep{Name: "b", Run: func(context.Context) error { b.Add(1); return nil }},
	)
	s.fire(context.Background())
	if a.Load() != 1 || b.Load() != 1 {
		t.Fatalf("ran a=%d b=%d, want each exactly once", a.Load(), b.Load())
	}
}

// A sweep returning an error must not stop the others, and fire must still
// return normally.
func TestFireContinuesPastAnErroringSweep(t *testing.T) {
	var ran atomic.Int32
	s := New(SweepHourUTC,
		Sweep{Name: "boom", Run: func(context.Context) error { return errors.New("nope") }},
		Sweep{Name: "ok", Run: func(context.Context) error { ran.Add(1); return nil }},
	)
	s.fire(context.Background())
	if ran.Load() != 1 {
		t.Fatalf("healthy sweep ran %d times, want 1 — an erroring sweep must not skip the rest", ran.Load())
	}
}

// A panicking sweep is recovered: the other sweeps still run and fire returns.
func TestFireRecoversFromAPanickingSweep(t *testing.T) {
	var ran atomic.Int32
	s := New(SweepHourUTC,
		Sweep{Name: "panic", Run: func(context.Context) error { panic("kaboom") }},
		Sweep{Name: "ok", Run: func(context.Context) error { ran.Add(1); return nil }},
	)
	done := make(chan struct{})
	go func() { s.fire(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fire did not return after a sweep panicked")
	}
	if ran.Load() != 1 {
		t.Fatalf("healthy sweep ran %d times, want 1 — a panicking sweep must not skip the rest", ran.Load())
	}
}

// The sweeps run concurrently, not one after another: both must be in-flight
// at the same moment.
func TestFireRunsSweepsConcurrently(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)
	entered := make(chan struct{}, 2)
	block := func(context.Context) error {
		entered <- struct{}{}
		wg.Done()
		wg.Wait() // returns only once both sweeps have entered
		return nil
	}
	s := New(SweepHourUTC,
		Sweep{Name: "a", Run: block},
		Sweep{Name: "b", Run: block},
	)
	done := make(chan struct{})
	go func() { s.fire(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fire deadlocked — sweeps are running sequentially, not concurrently")
	}
}

// Run does not fire a sweep at startup, and returns promptly when ctx is
// cancelled.
func TestRunDoesNotFireAtStartupAndStopsOnCancel(t *testing.T) {
	var ran atomic.Int32
	s := New(SweepHourUTC,
		Sweep{Name: "a", Run: func(context.Context) error { ran.Add(1); return nil }},
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx was cancelled")
	}
	if ran.Load() != 0 {
		t.Fatalf("sweep ran %d times before the target hour — Run must not fire at startup", ran.Load())
	}
}

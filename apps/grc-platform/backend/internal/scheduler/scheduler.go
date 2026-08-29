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

// Package scheduler runs the process's background sweeps on one shared daily
// timer.
//
// It replaces the two per-job tickers that used to live in
// internal/risk/job.EscalationJob.Start and internal/audit/job.ReminderJob.Start:
// having one loop means a single env switch (SCHEDULER_ENABLED, see
// internal/config) turns every sweep on or off together, and every sweep fires
// at the same predictable wall-clock time instead of one being 24h-since-boot.
//
// Each sweep still owns its own per-run timeout, de-dup, and error handling —
// this package only decides when they run, not what they do.
package scheduler

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// SweepHourUTC is the hour (UTC) every registered sweep fires at. UTC to match
// how the database's own DATETIME columns are pinned (see internal/db), and a
// fixed wall-clock hour rather than an interval so time-sensitive output — the
// audit digest email above all — lands at the same time regardless of when the
// server last restarted.
const SweepHourUTC = 8

// Sweep is one unit of scheduled work.
type Sweep struct {
	// Name identifies the sweep in log lines.
	Name string
	// Run performs one sweep synchronously and returns its error, if any. The
	// sweep is responsible for bounding its own runtime — the scheduler does
	// not impose a timeout — and for logging its own detail; the scheduler
	// logs only start/finish and a returned error.
	Run func(ctx context.Context) error
}

// Scheduler fires a fixed set of sweeps once per day at hourUTC:00 UTC.
type Scheduler struct {
	hourUTC int
	sweeps  []Sweep
}

// New builds a Scheduler that fires every sweep daily at hourUTC:00 UTC.
func New(hourUTC int, sweeps ...Sweep) *Scheduler {
	return &Scheduler{hourUTC: hourUTC, sweeps: sweeps}
}

// Run blocks until ctx is cancelled, firing every sweep once per day at
// hourUTC:00 UTC. It does not run a sweep at startup: the first fire is at the
// next occurrence of the target hour. Intended to be launched in its own
// goroutine from main.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		timer := time.NewTimer(durationUntilNext(s.hourUTC, time.Now().UTC()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.fire(ctx)
		}
	}
}

// fire runs every sweep concurrently and waits for all of them to return. The
// sweeps touch different modules, so running them in parallel is safe and
// keeps a slow one from delaying another; waiting for the batch means two
// daily fires can never overlap. A panicking sweep is recovered here so it
// neither skips the other sweeps nor takes the process down.
func (s *Scheduler) fire(ctx context.Context) {
	var wg sync.WaitGroup
	for _, sw := range s.sweeps {
		wg.Add(1)
		go func(sw Sweep) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("scheduler: sweep panicked",
						"sweep", sw.Name, "panic", r, "stack", string(debug.Stack()))
				}
			}()
			start := time.Now()
			slog.Info("scheduler: sweep started", "sweep", sw.Name)
			if err := sw.Run(ctx); err != nil {
				slog.Error("scheduler: sweep failed",
					"sweep", sw.Name, "err", err, "dur", time.Since(start))
				return
			}
			slog.Info("scheduler: sweep complete", "sweep", sw.Name, "dur", time.Since(start))
		}(sw)
	}
	wg.Wait()
}

// durationUntilNext returns the wait until the next occurrence of hour:00 UTC —
// today's if it hasn't passed yet, tomorrow's otherwise. A pure function of
// (hour, now) so it's unit-testable without mocking time.Now.
func durationUntilNext(hour int, now time.Time) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}

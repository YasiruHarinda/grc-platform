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

// Package job holds scheduled work that isn't triggered by an HTTP request.
//
// The overdue-escalation job used to live in the compliance-entity, on the
// reasoning that it is database-adjacent. It moved here because escalation grew
// two dependencies the entity does not and should not have: the HR entity, to
// resolve the assigner's and action owner's line managers, and the email
// service, to notify them. Leaving it there meant automatic escalations
// silently notified nobody while manual ones did — the two paths now run the
// same code, so they cannot drift.
package job

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// riskLister and escalator are the only two capabilities this job needs. They
// are declared here, narrowly, rather than taking the full RiskService and
// EscalationService: the concrete services satisfy them structurally, and a
// test can stand them up in a few lines instead of stubbing thirteen unrelated
// methods.
type riskLister interface {
	List(ctx context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error)
}

type escalator interface {
	Escalate(ctx context.Context, riskID int, createdBy string) (*model.Escalation, error)
}

// pageLimit is the page size used when walking overdue risks.
const pageLimit = 100

// runInterval is how often the job sweeps. Escalation is a daily-granularity
// concept — a risk is overdue by whole days — so anything finer would just be
// repeated no-ops.
const runInterval = 24 * time.Hour

// runTimeout bounds a single sweep. Each escalation does an HR lookup and an
// email send, so a large backlog takes real time; this exists to stop a wedged
// run from blocking every subsequent one, not to bound normal work.
const runTimeout = 30 * time.Minute

// EscalationJob finds IN_REMEDIATION risks whose implementation_date has passed
// and escalates them, notifying the same people a manual escalation would.
type EscalationJob struct {
	risks      riskLister
	escalation escalator
	// notify is called after each successful escalation. It is a function
	// rather than a handler dependency so this package doesn't import the
	// handler package (which imports services, and would cycle).
	notify func(ctx context.Context, riskID int, by string)
}

// escalatedBy is recorded as created_by on job-driven escalations, to
// distinguish them from a named user clicking Escalate.
const escalatedBy = "system"

// NewEscalationJob constructs the job. notify may be nil, in which case
// escalations still happen but nobody is emailed.
func NewEscalationJob(
	risks riskLister,
	escalation escalator,
	notify func(ctx context.Context, riskID int, by string),
) *EscalationJob {
	return &EscalationJob{risks: risks, escalation: escalation, notify: notify}
}

// Start runs the job once immediately, then every runInterval, until ctx is
// cancelled. Intended to be launched in its own goroutine from main.
func (j *EscalationJob) Start(ctx context.Context) {
	j.runOnce(ctx)

	ticker := time.NewTicker(runInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.runOnce(ctx)
		}
	}
}

// runOnce escalates every overdue IN_REMEDIATION risk it can find. A failure on
// one risk is logged and does not stop the rest — a transient error on one row
// shouldn't block the batch, and the next run picks up anything still overdue.
func (j *EscalationJob) runOnce(parent context.Context) {
	// This executes in a bare goroutine (see Start), where an unrecovered panic
	// would take the whole process down. Recover so a bad run is logged with
	// its stack and the ticker keeps scheduling future runs.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("escalation job: recovered from panic", "panic", r, "stack", string(debug.Stack()))
		}
	}()

	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()

	escalated, failed := 0, 0
	for {
		page, err := j.risks.List(ctx, model.ListRisksFilter{
			Statuses:       []string{model.StatusInRemediation},
			DueOverdueOnly: true,
			Limit:          pageLimit,
			// Offset stays 0 deliberately: each successful escalation moves the
			// risk to ESCALATED, dropping it out of this very result set. Paging
			// forward would step over the rows that shifted into the gap, so the
			// job re-queries from the start and stops when a page yields nothing
			// new. Failures are the only rows that persist, which is why the
			// loop also breaks when a whole page fails.
			Offset: 0,
		})
		if err != nil {
			slog.Error("escalation job: search overdue risks", "err", err)
			return
		}
		if len(page.Items) == 0 {
			break
		}

		progressed := false
		for _, r := range page.Items {
			// Escalate re-checks IN_REMEDIATION and overdue itself, so a risk
			// that moved on between this search and the call (someone closed
			// it, or a manual click got there first) is safely skipped.
			if _, err := j.escalation.Escalate(ctx, r.ID, escalatedBy); err != nil {
				slog.Warn("escalation job: risk", "riskId", r.ID, "err", err)
				failed++
				continue
			}
			escalated++
			progressed = true
			if j.notify != nil {
				j.notify(ctx, r.ID, escalatedBy)
			}
		}
		if !progressed {
			// Every row on this page failed; re-querying would return the same
			// rows forever. Stop and let the next run retry them.
			break
		}
	}
	slog.Info("escalation job: run complete", "escalated", escalated, "failed", failed)
}

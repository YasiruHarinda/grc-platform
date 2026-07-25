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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

// Package job holds background/scheduled work that isn't triggered by an
// HTTP request. The compliance-entity is the only component with direct
// database access, so this is where such jobs have to live.
package job

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/service"
)

// searchPageLimit is the page size used when paging through overdue risks;
// mirrors the pageLimit convention the GRC backend's entity-client uses when
// paging through this same search endpoint.
const searchPageLimit = 100

// EscalationJob finds IN_REMEDIATION risks whose implementation_date has
// passed and escalates them: creates a risk_escalation row and moves the risk
// to ESCALATED.
//
// Escalation here is fully automatic — no human supplies a target or reason
// (see risk_escalation's schema comment). Notifying Compliance/Management
// when this happens is intentionally not built yet: it needs an Asgardeo SCIM
// group lookup this deployment isn't subscribed to. Risks are still visible
// via the Overdue Risks tab regardless of whether anyone was notified.
type EscalationJob struct {
	riskSvc       service.RiskService
	escalationSvc service.RiskEscalationService
}

// NewEscalationJob constructs an EscalationJob.
func NewEscalationJob(riskSvc service.RiskService, escalationSvc service.RiskEscalationService) *EscalationJob {
	return &EscalationJob{riskSvc: riskSvc, escalationSvc: escalationSvc}
}

// Start runs the job once immediately, then again every 24 hours, until ctx
// is cancelled. Intended to be launched in its own goroutine from main.go.
func (j *EscalationJob) Start(ctx context.Context) {
	j.runOnce(ctx)

	ticker := time.NewTicker(24 * time.Hour)
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

// runOnce escalates every overdue IN_REMEDIATION risk it can find. A failure
// on one risk is logged and does not stop the rest — a transient error on one
// row shouldn't block the whole batch, and the next run will simply pick up
// anything still IN_REMEDIATION and overdue.
func (j *EscalationJob) runOnce(ctx context.Context) {
	// runOnce executes in a bare goroutine (see Start, launched from main.go),
	// where an unrecovered panic would crash the whole compliance-entity
	// process. Recover so a bad run is logged with its stack and the 24h
	// ticker keeps scheduling future runs.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("escalation job: recovered from panic: %v\n%s", r, debug.Stack())
		}
	}()

	escalated := 0
	offset := 0
	for {
		resp, err := j.riskSvc.SearchRisks(ctx, domain.SearchRisksRequest{
			WorkflowStatusKeys: []string{"IN_REMEDIATION"},
			DueOverdueOnly:     true,
			Pagination:         domain.Pagination{Limit: searchPageLimit, Offset: offset},
		})
		if err != nil {
			log.Printf("escalation job: search overdue risks (offset %d): %v", offset, err)
			return
		}
		escalatedThisPage := 0
		for _, r := range resp.Risks {
			// EscalateRisk re-checks IN_REMEDIATION+overdue itself, so a risk
			// that moved on between this search and the call (e.g. someone
			// just closed it, or a manual Escalate click already got there
			// first) is safely skipped rather than escalated out from under
			// whoever changed it.
			if _, err := j.escalationSvc.EscalateRisk(ctx, r.ID, domain.EscalateRiskRequest{CreatedBy: "system"}); err != nil {
				log.Printf("escalation job: risk %d: %v", r.ID, err)
				continue
			}
			escalated++
			escalatedThisPage++
		}
		if len(resp.Risks) < searchPageLimit {
			break
		}
		// Each successful escalation moves the risk to ESCALATED, dropping it
		// out of the IN_REMEDIATION+overdue result set for the next query. Only
		// the rows that stayed (failures) should advance the offset — advancing
		// by the full page would step past rows that shifted into the gap the
		// successes left, skipping them until the next scheduled run.
		offset += len(resp.Risks) - escalatedThisPage
	}
	log.Printf("escalation job: escalated %d risk(s)", escalated)
}

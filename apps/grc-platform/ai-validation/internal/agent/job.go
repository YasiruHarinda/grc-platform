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

package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/agent/mcpclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/agent/task"
)

// retryDelay is the pause before the single job-level retry (design §7).
const retryDelay = 30 * time.Second

// runner executes validation jobs: it writes the PENDING lifecycle row, runs
// the tool loop (with one retry on infrastructure failure), and writes an
// ERROR row if the job cannot produce a verdict. Terminal PASS/FAIL/UNCERTAIN
// rows are written by the MCP server when submit_validation_result succeeds.
type runner struct {
	mcp             *mcpclient.Client
	loop            *loop
	timeout         time.Duration
	sessionTTLGrace time.Duration
	log             *slog.Logger
}

// run is the entry point for one background job. It never returns an error to
// the caller — failures are recorded as ERROR rows and logged.
func (r *runner) run(spec task.TaskSpec, scope task.Scope, requestedBy, jobID string) {
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = r.timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log := r.log.With("jobId", jobID, "task", spec.Name,
		"auditId", scope.AuditID, "controlId", scope.ControlID, "evidenceId", scope.EvidenceID)
	log.Info("validation job started")

	// PENDING row so the UI can show "Analyzing…". Best-effort.
	if err := r.mcp.WriteLifecycle(ctx, scope.AuditID, scope.ControlID, scope.EvidenceID, "PENDING", ""); err != nil {
		log.Warn("could not write PENDING lifecycle row", "err", err)
	}

	submitted, err := r.attempt(ctx, spec, scope, requestedBy, log)
	if !submitted && err != nil && ctx.Err() == nil {
		// One retry with a fresh session for transient infrastructure failures.
		log.Warn("validation attempt failed; retrying once", "err", err)
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
		}
		if ctx.Err() == nil {
			submitted, err = r.attempt(ctx, spec, scope, requestedBy, log)
		}
	}

	if submitted {
		log.Info("validation job completed")
		return
	}

	summary := sanitizeError(ctx, err)
	log.Error("validation job failed", "reason", summary, "err", err)
	// Write ERROR with a detached context so a job-timeout can still record it.
	errCtx, errCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer errCancel()
	if werr := r.mcp.WriteLifecycle(errCtx, scope.AuditID, scope.ControlID, scope.EvidenceID, "ERROR", summary); werr != nil {
		log.Error("could not write ERROR lifecycle row", "err", werr)
	}
}

// attempt runs one full session: bootstrap → connect → tool loop.
func (r *runner) attempt(ctx context.Context, spec task.TaskSpec, scope task.Scope, requestedBy string, log *slog.Logger) (bool, error) {
	ttl := r.timeout + r.sessionTTLGrace
	token, err := r.mcp.CreateSession(ctx, spec.Name, scope.AuditID, scope.ControlID, scope.EvidenceID, requestedBy, ttl)
	if err != nil {
		return false, err
	}
	sess, err := r.mcp.Connect(ctx, token)
	if err != nil {
		return false, err
	}
	defer func() {
		if cerr := sess.Close(); cerr != nil {
			log.Debug("mcp session close", "err", cerr)
		}
	}()
	return r.loop.run(ctx, spec, scope, sess)
}

// sanitizeError maps a failure to a UI-safe summary — never raw provider errors,
// stack traces, or internal URLs (design §7).
func sanitizeError(ctx context.Context, err error) string {
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return "validation timed out"
	}
	if err == nil {
		return "AI could not complete the review"
	}
	return "AI service temporarily unavailable"
}

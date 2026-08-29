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
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// escalationJobHandler exposes a manual trigger for the daily overdue-risk
// escalation sweep (internal/risk/job.EscalationJob) — the direct counterpart
// of audit's reminderJobHandler. QA/ops convenience so the sweep can be
// re-run without waiting for its fixed daily time or restarting the server,
// and the only way to exercise the sweep on demand: unlike
// POST /api/v1/risks/{id}/escalate, which escalates one named risk, this runs
// the whole batch exactly as the scheduler does.
type escalationJobHandler struct {
	// trigger runs the sweep's full pass. A plain function (not a
	// job.EscalationJob field) so this package never imports internal/risk/job,
	// which would import back into handler and cycle — same reasoning as the
	// job.notify function field. Nil (job wiring not configured) answers 503.
	trigger func(ctx context.Context) error
	// running turns a concurrent second trigger into a synchronous 409 (below)
	// instead of a 202 followed by a run that silently fails. trigger is
	// EscalationJob.RunOnce, which has its own in-flight guard and would
	// reject the overlap — but only inside the detached goroutine, after this
	// handler has already written 202. Checking here lets the caller find out.
	running atomic.Bool
}

// run handles POST /api/v1/risks/escalations/run.
//
// The sweep runs detached in a goroutine rather than inline: the server's
// WriteTimeout is 30s, but a sweep is allowed up to 30 minutes
// (job.runTimeout), so running it inline could fail the response — or get
// cut off mid-run — long before the sweep itself finishes. The handler
// instead kicks the sweep off and answers 202 immediately; running guards
// against a retry (from that same timeout-driven failure) starting an
// overlapping second sweep.
func (h *escalationJobHandler) run(w http.ResponseWriter, r *http.Request) {
	// ManageRiskHub, not RISK_ESCALATE. This fires a sweep that escalates
	// overdue risks and emails management across EVERY register. RISK_ESCALATE
	// is register-scoped — the single-risk endpoint gates it with
	// RequirePrivilegeIn(..., sourceRegisterID) — so an unscoped RISK_ESCALATE
	// check here would let someone holding it in one register act hub-wide.
	// ManageRiskHub is the GLOBAL-only, platform-admin privilege every other
	// Risk Hub bulk/admin route already gates on; it is the risk-side
	// equivalent of the audit reminder trigger's ManageControls gate.
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageRiskHub) {
		return
	}
	if h.trigger == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "escalation job is not configured")
		return
	}
	if !h.running.CompareAndSwap(false, true) {
		response.WriteError(w, http.StatusConflict, "escalation job is already running")
		return
	}
	go func() { // #nosec G118 -- deliberately detached from r.Context(): it would cancel this sweep the instant the handler returns 202, well before the up-to-30min run finishes
		defer h.running.Store(false)
		defer func() {
			if p := recover(); p != nil {
				slog.Error("escalation job: manual trigger panic", "panic", p)
			}
		}()
		if err := h.trigger(context.Background()); err != nil {
			slog.Error("escalation job: manual trigger failed", "err", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

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

// reminderJobHandler exposes a manual trigger for the daily due-date
// reminder digest (internal/audit/job.ReminderJob) — QA/ops convenience so
// the job can be tested/re-run without waiting for its fixed daily time or
// restarting the server.
type reminderJobHandler struct {
	// trigger runs the job's full sweep. A plain function (not a
	// job.ReminderJob field) so this package never imports internal/audit/job,
	// which would import back into handler and cycle — same reasoning as
	// risk's job.notify function field. Nil (job wiring not configured)
	// answers 503.
	trigger func(ctx context.Context) error
	// running guards against a second manual trigger starting a sweep while
	// one is already in flight — the sweep itself (job.runOnce) has no such
	// guard, and one sweep can take up to 30 minutes (job.runTimeout).
	running atomic.Bool
}

// run handles POST /api/v1/audits/reminders/run.
//
// The sweep runs detached in a goroutine rather than inline: the server's
// WriteTimeout is 30s, but a sweep is allowed up to 30 minutes
// (job.runTimeout), so running it inline could fail the response — or get
// cut off mid-run — long before the sweep itself finishes. The handler
// instead kicks the sweep off and answers 202 immediately; running guards
// against a retry (from that same timeout-driven failure) starting an
// overlapping second sweep.
func (h *reminderJobHandler) run(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageControls) {
		return
	}
	if h.trigger == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "reminder job is not configured")
		return
	}
	if !h.running.CompareAndSwap(false, true) {
		response.WriteError(w, http.StatusConflict, "reminder job is already running")
		return
	}
	go func() {
		defer h.running.Store(false)
		defer func() {
			if p := recover(); p != nil {
				slog.Error("reminder job: manual trigger panic", "panic", p)
			}
		}()
		if err := h.trigger(context.Background()); err != nil {
			slog.Error("reminder job: manual trigger failed", "err", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

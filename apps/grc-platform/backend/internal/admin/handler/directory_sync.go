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
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// directorySyncHandler is the manual trigger, and the intended way to verify the
// sync against a real environment. Works with the scheduler off.
type directorySyncHandler struct {
	// A plain function so this package never imports the job's construction.
	// It claims the job's run slot and reports false if a run is already in
	// flight; nil (job wiring not configured) answers 503.
	trigger     func() bool
	activityLog *adminactivity.Client
}

// run handles POST /api/v1/admin/directory-sync/run. Answers 202 without the
// counts — a run outlasts the 30s write timeout — and logs them instead.
func (h *directorySyncHandler) run(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.ManageUsers) {
		return
	}
	if h.trigger == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "directory status sync is not configured")
		return
	}
	// Claimed before replying, and claimed on the job itself: the scheduled
	// sweep shares it, so a second flag here would answer 202 to a trigger the
	// job then refuses out of sight.
	if !h.trigger() {
		response.WriteError(w, http.StatusConflict, "directory status sync is already running")
		return
	}

	// The log's entity_type enum has no value for a system job, so the run is
	// recorded against the admin who fired it, with details naming what ran.
	callerID := 0
	if caller := auth.FromContext(r.Context()); caller != nil {
		callerID = caller.UserID
	}
	h.activityLog.Log(r.Context(), actor(r), adminactivity.ActionUpdated, adminactivity.EntityUser, callerID,
		map[string]any{"job": "Directory Status Sync"})

	w.WriteHeader(http.StatusAccepted)
}

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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

func postToEscalationRun(t *testing.T, h *escalationJobHandler, grants map[string]bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/escalations/run", nil)
	req = req.WithContext(contextForGrants(t, grants, nil))
	rr := httptest.NewRecorder()
	h.run(rr, req)
	return rr
}

// Without RISK_ESCALATE the sweep must not be reachable, and the trigger must
// never be called.
func TestEscalationJobRun_RequiresEscalatePrivilege(t *testing.T) {
	called := false
	h := &escalationJobHandler{trigger: func(context.Context) error { called = true; return nil }}

	rr := postToEscalationRun(t, h, map[string]bool{privilege.ViewRisks: true})

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if called {
		t.Error("trigger was called despite the caller lacking RISK_ESCALATE")
	}
}

// A nil trigger (job wiring not configured) answers 503, not 202.
func TestEscalationJobRun_NotConfigured(t *testing.T) {
	h := &escalationJobHandler{trigger: nil}

	rr := postToEscalationRun(t, h, map[string]bool{privilege.EscalateRisk: true})

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

// The happy path answers 202 immediately and runs the sweep detached.
func TestEscalationJobRun_Accepts202AndRunsSweep(t *testing.T) {
	ran := make(chan struct{}, 1)
	h := &escalationJobHandler{trigger: func(context.Context) error { ran <- struct{}{}; return nil }}

	rr := postToEscalationRun(t, h, map[string]bool{privilege.EscalateRisk: true})

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rr.Code)
	}
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("trigger was never invoked after a 202")
	}
}

// A second trigger while a sweep is still in flight gets 409, and the running
// sweep is left alone.
func TestEscalationJobRun_ConflictWhileRunning(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	h := &escalationJobHandler{trigger: func(context.Context) error {
		started <- struct{}{}
		<-release
		return nil
	}}

	first := postToEscalationRun(t, h, map[string]bool{privilege.EscalateRisk: true})
	if first.Code != http.StatusAccepted {
		t.Fatalf("first call status = %d, want 202", first.Code)
	}
	<-started // the sweep goroutine is now inside trigger, holding running

	second := postToEscalationRun(t, h, map[string]bool{privilege.EscalateRisk: true})
	if second.Code != http.StatusConflict {
		t.Fatalf("second call status = %d, want 409", second.Code)
	}

	close(release)
}

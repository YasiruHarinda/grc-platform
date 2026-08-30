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

// External test package: the handler packages import routeguard, so an
// in-package test importing them back would be an import cycle.
package routeguard_test

import (
	"net/http"
	"sort"
	"testing"

	adminhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/admin/handler"
	audithandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/handler"
	riskhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/handler"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/routeguard"
	userhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user/handler"
)

// recordingRouter captures every pattern registered against it.
type recordingRouter struct{ patterns []string }

func (r *recordingRouter) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
}

// registeredPatterns runs every RegisterRoutes against a recorder. Zero-valued
// Deps are safe: registration only takes method addresses, none of them run.
// GET /health is not covered — it is registered in main and exempted in Auth.
func registeredPatterns(t *testing.T) []string {
	t.Helper()
	rec := &recordingRouter{}
	userhandler.RegisterRoutes(rec, userhandler.Deps{})
	riskhandler.RegisterRoutes(rec, riskhandler.Deps{})
	audithandler.RegisterRoutes(rec, audithandler.Deps{})
	adminhandler.RegisterRoutes(rec, adminhandler.Deps{})
	if len(rec.patterns) == 0 {
		t.Fatal("no routes recorded; RegisterRoutes no longer registers through the Router interface")
	}
	return rec.patterns
}

// TestEveryRouteIsClassified is the drift guard: an unclassified route would be
// invisible to external callers by accident rather than by decision.
func TestEveryRouteIsClassified(t *testing.T) {
	classified := routeguard.Patterns()
	var missing []string
	for _, p := range registeredPatterns(t) {
		if _, ok := classified[p]; !ok {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	for _, p := range missing {
		t.Errorf("route %q is not classified; add it to externalVisible as true or false", p)
	}
}

// TestNoStaleClassifications asserts the other direction. A stale entry is
// harmless alone but survives a rename, where the obvious fix is to copy the
// old entry — true and all. This makes a rename look like a rename.
func TestNoStaleClassifications(t *testing.T) {
	live := make(map[string]struct{})
	for _, p := range registeredPatterns(t) {
		live[p] = struct{}{}
	}
	var stale []string
	for p := range routeguard.Patterns() {
		if _, ok := live[p]; !ok {
			stale = append(stale, p)
		}
	}
	sort.Strings(stale)
	for _, p := range stale {
		t.Errorf("classified route %q is not registered anywhere; drop it or fix the pattern", p)
	}
}

// TestDuplicateRegistration catches what the real mux would panic on. The
// recorder does not, so the drift tests would otherwise pass on a route set the
// server cannot start with.
func TestDuplicateRegistration(t *testing.T) {
	seen := make(map[string]struct{})
	for _, p := range registeredPatterns(t) {
		if _, dup := seen[p]; dup {
			t.Errorf("route %q registered more than once", p)
		}
		seen[p] = struct{}{}
	}
}

// TestExternalVisibleUnknownPatternDenies pins the default. The mux returns ""
// for an unmatched path and a method mismatch; neither is permission.
func TestExternalVisibleUnknownPatternDenies(t *testing.T) {
	for _, p := range []string{"", "GET /api/v1/nope", "DELETE /api/v1/me/profile"} {
		if routeguard.ExternalVisible(p) {
			t.Errorf("ExternalVisible(%q) = true, want false", p)
		}
	}
}

// TestRiskAndAdminAreNeverExternallyVisible catches a mistyped `true` deep in
// the Risk block, which reads as just another line in a long map.
func TestRiskAndAdminAreNeverExternallyVisible(t *testing.T) {
	for pattern, visible := range routeguard.Patterns() {
		if !visible {
			continue
		}
		for _, prefix := range []string{"/api/v1/risks", "/api/v1/admin"} {
			if idx := indexOfPath(pattern); idx >= 0 && hasPrefix(pattern[idx:], prefix) {
				t.Errorf("route %q is externally visible; no Risk Hub or Admin route may be", pattern)
			}
		}
	}
}

// indexOfPath returns where the path starts in a "METHOD /path" pattern.
func indexOfPath(pattern string) int {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '/' {
			return i
		}
	}
	return -1
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

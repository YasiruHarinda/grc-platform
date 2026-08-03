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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// contextFor builds a context carrying a resolved privilege set the way the
// Auth middleware would, for the given role. Exercises the exact path
// isActionOwnerOnly reads (middleware.UserInfoFromContext + privilege.FromContext)
// without needing a live server or a signed JWT — AUTH_TOKEN_VALIDATOR_ENABLED=false
// skips loading a real privilege.Store entirely (see cmd/server/main.go),
// making this the only way to test privilege-gated behaviour without a real IdP.
func contextFor(t *testing.T, role string) context.Context {
	t.Helper()
	store := privilege.NewForTest(map[string]map[string]bool{
		"grc-platform-risk-action-owner": {
			privilege.ViewRisks:           true,
			privilege.CompleteActionSteps: true,
		},
		"grc-platform-management": {
			privilege.ViewRisks:                  true,
			privilege.ManagementApproveRisk:      true,
			privilege.CreateManagementActionPlan: true,
		},
		"grc-platform-risk-compliance-admin": {
			privilege.ViewRisks:  true,
			privilege.CreateRisk: true,
			// ComplianceApproveRisk is what canOverrideAssignee tests for, and
			// the real seed grants it to this role alone — see
			// shared_seed_data.sql's role_privilege block.
			privilege.ComplianceApproveRisk: true,
			privilege.CompleteActionSteps:   true,
		},
		"grc-platform-risk-assigner": {
			privilege.ViewRisks:  true,
			privilege.CreateRisk: true,
		},
		"grc-platform-risk-owner": {
			privilege.ViewRisks:        true,
			privilege.OwnerApproveRisk: true,
		},
		"grc-platform-compliance-team": {
			privilege.ViewRisks:    true,
			privilege.ViewAllRisks: true,
		},
	})
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Email: "test@wso2.com"})
	return privilege.WithContext(ctx, store.Resolve([]string{role}))
}

func TestIsActionOwnerOnly(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{"grc-platform-risk-action-owner", true},
		{"grc-platform-management", false},
		{"grc-platform-risk-compliance-admin", false}, // holds CompleteActionSteps AND CreateRisk
	}
	for _, c := range cases {
		got := isActionOwnerOnly(contextFor(t, c.role))
		if got != c.want {
			t.Errorf("isActionOwnerOnly(%s) = %v, want %v", c.role, got, c.want)
		}
	}
}

func TestIsTeamScopedOnly(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{"grc-platform-risk-assigner", true},
		{"grc-platform-risk-owner", true},
		{"grc-platform-compliance-team", false}, // holds ViewAllRisks, in seesEveryRisk
		{"grc-platform-management", false},      // holds ManagementApproveRisk, in seesEveryRisk
		// Action-Owner-only must never also be classified as team-scoped —
		// isTeamScopedOnly explicitly excludes it so the two never overlap.
		{"grc-platform-risk-action-owner", false},
	}
	for _, c := range cases {
		got := isTeamScopedOnly(contextFor(t, c.role))
		if got != c.want {
			t.Errorf("isTeamScopedOnly(%s) = %v, want %v", c.role, got, c.want)
		}
	}
}

// fakeUserRepo resolves exactly one email to one id, so requireRiskActor can be
// exercised without the Compliance Entity. Every other method is unused here.
type fakeUserRepo struct {
	email string
	id    int
}

func (f fakeUserRepo) GetByEmail(_ context.Context, email string) (*user.User, error) {
	if email != f.email {
		return nil, nil // "not found" is a domain condition, not an error
	}
	return &user.User{ID: f.id, Email: email}, nil
}
func (f fakeUserRepo) GetByID(context.Context, int) (*user.User, error) { return nil, nil }
func (f fakeUserRepo) Upsert(context.Context, string, string, string) (*user.User, error) {
	return nil, nil
}
func (f fakeUserRepo) List(context.Context) ([]*user.User, error) { return nil, nil }

func TestCanOverrideAssignee(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		// Only the compliance admin may act in another user's place.
		{"grc-platform-risk-compliance-admin", true},
		{"grc-platform-risk-owner", false},
		{"grc-platform-risk-assigner", false},
		{"grc-platform-management", false},
		{"grc-platform-risk-action-owner", false},
		// Read-only: sees every risk, but that must not imply acting on one.
		{"grc-platform-compliance-team", false},
	}
	for _, c := range cases {
		if got := canOverrideAssignee(contextFor(t, c.role)); got != c.want {
			t.Errorf("canOverrideAssignee(%s) = %v, want %v", c.role, got, c.want)
		}
	}
}

func TestRequireRiskActor(t *testing.T) {
	const callerEmail = "test@wso2.com" // the email contextFor puts on the request
	cases := []struct {
		name       string
		role       string
		callerID   int // id the caller's email resolves to; 0 = no platform user row
		wantUserID int // the id named on the risk
		wantOK     bool
		wantStatus int
	}{
		{"named actor passes", "grc-platform-risk-owner", 7, 7, true, http.StatusOK},
		{"different user is refused", "grc-platform-risk-owner", 7, 9, false, http.StatusForbidden},
		// The override is the whole point of the compliance-admin escape hatch:
		// a risk whose named owner has left must not deadlock.
		{"compliance admin overrides a mismatch", "grc-platform-risk-compliance-admin", 7, 9, true, http.StatusOK},
		// No platform user row can never match, so it must refuse rather than
		// fall through — nil is not "any user".
		{"caller with no user row is refused", "grc-platform-risk-owner", 0, 9, false, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := fakeUserRepo{email: callerEmail, id: c.callerID}
			if c.callerID == 0 {
				repo.email = "someone-else@wso2.com" // caller's email resolves to nothing
			}
			d := &Deps{Users: repo}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/1/owner-approve", nil).
				WithContext(contextFor(t, c.role))
			rec := httptest.NewRecorder()

			got := d.requireRiskActor(rec, req, c.wantUserID, "Risk Owner")
			if got != c.wantOK {
				t.Errorf("requireRiskActor = %v, want %v", got, c.wantOK)
			}
			if !c.wantOK && rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, c.wantStatus)
			}
			if c.wantOK && rec.Code != http.StatusOK {
				t.Errorf("passing check wrote status %d, want nothing written", rec.Code)
			}
		})
	}
}

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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// fakeUsersListRepo implements user.Repository, returning fixed rows from List
// and failing every other method — handleListUsers never calls them. Distinct
// from risk_registers_test.go's fakeUserRepo, which is built around a single
// uuid→id resolution and returns nothing from List.
type fakeUsersListRepo struct {
	users []*user.User
}

func (f *fakeUsersListRepo) List(context.Context) ([]*user.User, error) { return f.users, nil }
func (f *fakeUsersListRepo) GetByID(context.Context, int) (*user.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUsersListRepo) GetByUUID(context.Context, string) (*user.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUsersListRepo) Upsert(context.Context, string, string) (*user.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUsersListRepo) UpsertTyped(context.Context, string, string, string) (*user.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUsersListRepo) UpdateStatus(context.Context, int, string, string) (*user.User, error) {
	panic("not used by handleListUsers")
}

// TestHandleListUsers_LocalDevWithoutDirectory covers the review comment: with
// no SCIM configured (Directory == nil), the handler used to always return [],
// unlike candidates.go's resolveCandidates, which keeps its list full in
// local dev via keepUnresolved. Local dev must offer every row here too.
func TestHandleListUsers_LocalDevWithoutDirectory(t *testing.T) {
	repo := &fakeUsersListRepo{users: []*user.User{
		{ID: 1, UUID: "uuid-1", Email: "person1@wso2.com", DisplayName: "Person One", Status: "ACTIVE"},
		{ID: 2, UUID: "uuid-2", Email: "person2@wso2.com", DisplayName: "Person Two", Status: "ACTIVE"},
	}}

	d := &Deps{Users: repo, Directory: nil} // Directory == nil: no SCIM configured

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/users", nil)
	// Local dev signal for auth.AllowAll: a UserInfo present, no privilege
	// store configured (privilege.FromContext stays nil by not setting one).
	req = req.WithContext(middleware.WithUserInfo(req.Context(), &middleware.UserInfo{Subject: "dev-user"}))
	rr := httptest.NewRecorder()

	d.handleListUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got []*user.User
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != len(repo.users) {
		t.Fatalf("got %d users, want %d (local dev must not empty this list) — body: %s", len(got), len(repo.users), rr.Body.String())
	}
}

// TestHandleListUsers_NoDirectoryOutsideLocalDev covers the complementary
// case: an authenticated, privileged caller in a real deployment whose
// directory happens to be unconfigured (should not happen, but must not
// silently leak unresolved rows either) — auth.AllowAll is false because the
// privilege map is a real (non-nil) grant, not local dev's unset map.
func TestHandleListUsers_NoDirectoryOutsideLocalDev(t *testing.T) {
	repo := &fakeUsersListRepo{users: []*user.User{
		{ID: 1, UUID: "uuid-1", Email: "person1@wso2.com", DisplayName: "Person One", Status: "ACTIVE"},
	}}

	d := &Deps{Users: repo, Directory: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/users", nil)
	req = req.WithContext(contextForGrants(t, map[string]bool{privilege.ViewRisks: true}, nil))
	rr := httptest.NewRecorder()

	d.handleListUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	var got []*user.User
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d users, want 0 (not local dev, no directory to resolve through)", len(got))
	}
}

// TestHandleListUsers_RequiresPrivilege is the regression test for the
// missing-authorization finding: GET /api/v1/risks/users had no privilege
// check at all, so any authenticated caller — an external auditor included —
// could enumerate every active platform user's resolved name and email.
func TestHandleListUsers_RequiresPrivilege(t *testing.T) {
	repo := &fakeUsersListRepo{users: []*user.User{
		{ID: 1, UUID: "uuid-1", Email: "person1@wso2.com", DisplayName: "Person One", Status: "ACTIVE"},
	}}

	d := &Deps{Users: repo, Directory: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/risks/users", nil)
	// Authenticated, but holding a privilege from an unrelated module (Audit
	// Hub) — e.g. an external auditor — not RISK_VIEW_RISKS/MANAGE_RISK_HUB.
	req = req.WithContext(contextForGrants(t, map[string]bool{privilege.ViewAudits: true}, nil))
	rr := httptest.NewRecorder()

	d.handleListUsers(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

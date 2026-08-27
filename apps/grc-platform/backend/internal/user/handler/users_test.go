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
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// authedCtx builds a context the way the Auth middleware would for an
// authenticated caller holding exactly the given privileges — a non-nil
// privilege map, so auth.AllowAll(ctx) is false (unlike the true local-dev
// signal used by TestHandleListUsers_LocalDevWithoutDirectory below, which
// deliberately leaves the privilege map unset).
func authedCtx(privs map[string]bool) context.Context {
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Subject: "test-caller-uuid"})
	return privilege.WithContext(ctx, privs)
}

// fakeUserRepo implements userentity.Repository, returning fixed rows from
// List and failing every other method — this handler never calls them.
type fakeUserRepo struct {
	users []*userentity.User
}

func (f *fakeUserRepo) List(ctx context.Context) ([]*userentity.User, error) { return f.users, nil }
func (f *fakeUserRepo) GetByID(ctx context.Context, id int) (*userentity.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUserRepo) GetByUUID(ctx context.Context, uuid string) (*userentity.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUserRepo) Upsert(ctx context.Context, uuid, actor string) (*userentity.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUserRepo) UpsertTyped(ctx context.Context, uuid, userType, actor string) (*userentity.User, error) {
	panic("not used by handleListUsers")
}
func (f *fakeUserRepo) UpdateStatus(ctx context.Context, id int, status, actor string) (*userentity.User, error) {
	panic("not used by handleListUsers")
}

// TestHandleListUsers_LocalDevWithoutDirectory covers the review comment: with
// no SCIM configured (dir == nil), the handler used to always return [],
// unlike candidates.go's resolveCandidates, which keeps its list full in
// local dev via keepUnresolved. Local dev must offer every row here too.
func TestHandleListUsers_LocalDevWithoutDirectory(t *testing.T) {
	repo := &fakeUserRepo{users: []*userentity.User{
		{ID: 1, UUID: "uuid-1", Email: "person1@wso2.com", DisplayName: "Person One", Status: "ACTIVE"},
		{ID: 2, UUID: "uuid-2", Email: "person2@wso2.com", DisplayName: "Person Two", Status: "ACTIVE"},
	}}

	h := handleListUsers(repo, nil) // dir == nil: no SCIM configured

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	// Local dev signal for auth.AllowAll: a UserInfo present, no privilege
	// store configured (privilege.FromContext stays nil by not setting one).
	req = req.WithContext(middleware.WithUserInfo(req.Context(), &middleware.UserInfo{Subject: "dev-user"}))
	rr := httptest.NewRecorder()

	h(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got []*userentity.User
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
	repo := &fakeUserRepo{users: []*userentity.User{
		{ID: 1, UUID: "uuid-1", Email: "person1@wso2.com", DisplayName: "Person One", Status: "ACTIVE"},
	}}

	h := handleListUsers(repo, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req = req.WithContext(authedCtx(map[string]bool{privilege.ViewRisks: true}))
	rr := httptest.NewRecorder()

	h(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	var got []*userentity.User
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d users, want 0 (not local dev, no directory to resolve through)", len(got))
	}
}

// TestHandleListUsers_RequiresPrivilege is the regression test for the
// missing-authorization finding: GET /api/v1/users had no privilege check at
// all, so any authenticated caller — an external auditor included — could
// enumerate every active platform user's resolved name and email.
func TestHandleListUsers_RequiresPrivilege(t *testing.T) {
	repo := &fakeUserRepo{users: []*userentity.User{
		{ID: 1, UUID: "uuid-1", Email: "person1@wso2.com", DisplayName: "Person One", Status: "ACTIVE"},
	}}

	h := handleListUsers(repo, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	// Authenticated, but holding a privilege from an unrelated module (Audit
	// Hub) — e.g. an external auditor — not RISK_VIEW_RISKS/MANAGE_RISK_HUB.
	req = req.WithContext(authedCtx(map[string]bool{privilege.ViewAudits: true}))
	rr := httptest.NewRecorder()

	h(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

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

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	auditentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository/entity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	portalhandler "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/portal/handler"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/grant"
)

type stubGrants struct{ caller grant.Caller }

func (s stubGrants) ForUUID(context.Context, string) (grant.Caller, error) { return s.caller, nil }
func (s stubGrants) Candidates(context.Context, string, []int) ([]grant.Candidate, error) {
	return nil, nil
}
func (s stubGrants) CreateGrant(context.Context, int, grant.CreateGrantRequest) (grant.Grant, error) {
	return grant.Grant{}, nil
}
func (s stubGrants) RevokeGrant(context.Context, int, int, string) error { return nil }

// entityStub serves POST /controls/search with a single canned control.
func entityStub(t *testing.T, control map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/controls/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := map[string]any{"controls": []any{}}
		if control != nil {
			resp["controls"] = []any{control}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func multipartBody(t *testing.T, email string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if email != "" {
		_ = mw.WriteField("email", email)
	}
	fw, err := mw.CreateFormFile("file", "shot.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("\x89PNG\r\n\x1a\nfake"))
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func postWithCaller(t *testing.T, h http.Handler, teamID int, body *bytes.Buffer, ct string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/evidence-portal/controls/811/evidences", body)
	r.Header.Set("Content-Type", ct)
	r = r.WithContext(middleware.WithPortalCaller(context.Background(), &middleware.PortalCaller{ClientID: "c1", TeamID: teamID}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

func newMux(deps portalhandler.Deps) *http.ServeMux {
	mux := http.NewServeMux()
	portalhandler.RegisterRoutes(mux, deps)
	return mux
}

func TestSubmitEvidence_WrongTeamIs404(t *testing.T) {
	srv := entityStub(t, map[string]any{"id": 811, "auditId": 42, "teamId": 7, "status": "EVIDENCE_PENDING"})
	mux := newMux(portalhandler.Deps{Controls: auditentity.NewPortalControlReader(entityclient.New(srv.URL))})

	body, ct := multipartBody(t, "person@wso2.com")
	rr := postWithCaller(t, mux, 99 /* not team 7 */, body, ct)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestSubmitEvidence_NullTeamIs404(t *testing.T) {
	srv := entityStub(t, map[string]any{"id": 811, "auditId": 42, "status": "EVIDENCE_PENDING"}) // no teamId
	mux := newMux(portalhandler.Deps{Controls: auditentity.NewPortalControlReader(entityclient.New(srv.URL))})

	body, ct := multipartBody(t, "person@wso2.com")
	rr := postWithCaller(t, mux, 7, body, ct)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestSubmitEvidence_SamplePhaseStatusIs409(t *testing.T) {
	srv := entityStub(t, map[string]any{"id": 811, "auditId": 42, "teamId": 7, "status": "SUBMITTED_SAMPLE"})
	mux := newMux(portalhandler.Deps{Controls: auditentity.NewPortalControlReader(entityclient.New(srv.URL))})

	body, ct := multipartBody(t, "person@wso2.com")
	rr := postWithCaller(t, mux, 7, body, ct)
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rr.Code)
	}
}

func TestSubmitEvidence_UnresolvableEmailIs400(t *testing.T) {
	srv := entityStub(t, map[string]any{"id": 811, "auditId": 42, "teamId": 7, "status": "EVIDENCE_PENDING"})
	mux := newMux(portalhandler.Deps{
		Controls:  auditentity.NewPortalControlReader(entityclient.New(srv.URL)),
		Directory: directory.New(nil, 0), // nil SCIM: every ResolveEmail is unresolved
		Grants:    stubGrants{},
	})

	body, ct := multipartBody(t, "ghost@wso2.com")
	rr := postWithCaller(t, mux, 7, body, ct)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestListControls_RequiresCaller(t *testing.T) {
	mux := newMux(portalhandler.Deps{})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/evidence-portal/controls", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without a portal caller, got %d", rr.Code)
	}
}

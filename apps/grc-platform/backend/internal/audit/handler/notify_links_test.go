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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/audit/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/applink"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
)

const (
	linkTestInternalUUID = "11111111-1111-1111-1111-111111111111"
	linkTestExternalUUID = "22222222-2222-2222-2222-222222222222"
)

// linkTestUsers serves the two recipients; every other UserRepository method is
// left to the embedded nil interface, which these tests never reach.
type linkTestUsers struct {
	repository.UserRepository
	byID map[int]*model.UserRef
}

func (u linkTestUsers) GetByID(_ context.Context, id int) (*model.UserRef, error) {
	return u.byID[id], nil
}

// linkTestServer plays both the SCIM directory (one org per user type) and the
// email service, recording each sent email's rendered HTML body.
type linkTestServer struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newLinkTestServer(t *testing.T) *linkTestServer {
	t.Helper()
	s := &linkTestServer{}
	mux := http.NewServeMux()
	token := func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	}
	mux.HandleFunc("POST /oauth2/token", token)
	search := func(uuid, email string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Filter string `json:"filter"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			res := []map[string]any{}
			if strings.Contains(in.Filter, uuid) {
				res = append(res, map[string]any{"id": uuid, "userName": email, "name": map[string]any{"givenName": "Test", "familyName": "User"}})
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"Resources": res})
		}
	}
	mux.HandleFunc("POST /t/internal/scim2/Users/.search", search(linkTestInternalUUID, "insider@wso2.com"))
	mux.HandleFunc("POST /t/external/scim2/Users/.search", search(linkTestExternalUUID, "auditor@partner.example"))
	mux.HandleFunc("POST /send-email", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Template string `json:"template"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		html, _ := base64.StdEncoding.DecodeString(in.Template)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(html))
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *linkTestServer) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

func (s *linkTestServer) deps() *Deps {
	scimClient := func(org string) *scim.Client {
		return scim.NewClient(s.srv.URL, s.srv.URL+"/oauth2/token", "id", "secret", "scope", org)
	}
	return &Deps{
		Users: linkTestUsers{byID: map[int]*model.UserRef{
			1: {ID: 1, UUID: linkTestInternalUUID, UserType: "INTERNAL", Status: "ACTIVE"},
			2: {ID: 2, UUID: linkTestExternalUUID, UserType: "EXTERNAL", Status: "ACTIVE"},
		}},
		Directory: directory.NewWithExternal(scimClient("internal"), scimClient("external"), time.Hour, time.Hour),
		Email:     emailer.New(s.srv.URL, "grc@example.com", s.srv.URL+"/oauth2/token", "id", "secret", true),
		Links:     applink.New("https://grc.example.com", "https://one.example.com"),
	}
}

// The same event, sent to an internal and an external recipient, must link to
// the host each of them can actually use.
func TestSendAuditEventLinksByRecipientType(t *testing.T) {
	tests := []struct {
		name        string
		recipientID int
		wantLink    string
		wantNot     string
	}{
		{"internal", 1, "https://one.example.com/security/audit/audits/7?control=3", "https://grc.example.com"},
		{"external", 2, "https://grc.example.com/audit/audits/7?control=3", "https://one.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newLinkTestServer(t)
			d := s.deps()
			info := emailer.AuditEventInfo{
				AuditName: "ISO 27001",
				DetailURL: applink.ControlPath(7, 3),
				Items:     []emailer.AuditEventItem{{ControlNumber: "A.1", DetailURL: applink.ControlPath(7, 3)}},
			}
			if err := d.sendAuditEvent(context.Background(), emailer.AuditEventSampleSubmitted, tt.recipientID, info, nil, true); err != nil {
				t.Fatalf("sendAuditEvent: %v", err)
			}
			got := s.sent()
			if len(got) != 1 {
				t.Fatalf("sent %d emails, want 1", len(got))
			}
			// html/template escapes "&" and "?" in hrefs only where needed; the
			// query string here has neither, so the link appears verbatim.
			if !strings.Contains(got[0], `href="`+tt.wantLink+`"`) {
				t.Errorf("email lacks link %q:\n%s", tt.wantLink, got[0])
			}
			if strings.Contains(got[0], tt.wantNot) {
				t.Errorf("email contains the other host %q", tt.wantNot)
			}
			if strings.Contains(got[0], `href="/`) {
				t.Error("email contains an unresolved relative link")
			}
		})
	}
}

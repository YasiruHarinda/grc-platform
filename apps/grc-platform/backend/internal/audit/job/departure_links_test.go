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

package job

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
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directorysync"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/applink"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
)

const departureTestUUID = "11111111-1111-1111-1111-111111111111"

type departureTestUsers map[int]*model.UserRef

func (u departureTestUsers) GetByID(_ context.Context, id int) (*model.UserRef, error) {
	return u[id], nil
}

// The departure digest goes to an admin; its footer and every row link must use
// the host that admin's user type calls for.
func TestDepartureNotifyLinksByRecipientType(t *testing.T) {
	tests := []struct {
		name     string
		userType string
		wantHost string
		wantNot  string
	}{
		{"internal admin", "INTERNAL", "https://one.example.com/security", "https://grc.example.com"},
		// An external admin resolves through the external org, which this
		// fake does not serve, so the unrecognised-type fallback is the case
		// worth pinning here instead.
		{"unrecognised type falls back", "", "https://grc.example.com", "https://one.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				mu   sync.Mutex
				sent []string
			)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
			})
			mux.HandleFunc("POST /t/internal/scim2/Users/.search", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"Resources": []map[string]any{{
					"id": departureTestUUID, "userName": "admin@wso2.com",
					"name": map[string]any{"givenName": "Admin", "familyName": "User"},
				}}})
			})
			mux.HandleFunc("POST /send-email", func(w http.ResponseWriter, r *http.Request) {
				var in struct {
					Template string `json:"template"`
				}
				_ = json.NewDecoder(r.Body).Decode(&in)
				html, _ := base64.StdEncoding.DecodeString(in.Template)
				mu.Lock()
				sent = append(sent, string(html))
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			hub := NewDepartureHub(DepartureDeps{
				Users: departureTestUsers{9: {ID: 9, UUID: departureTestUUID, UserType: tt.userType, Status: "ACTIVE"}},
				Directory: directory.New(
					scim.NewClient(srv.URL, srv.URL+"/oauth2/token", "id", "secret", "scope", "internal"), time.Hour),
				Email: emailer.New(srv.URL, "grc@example.com", srv.URL+"/oauth2/token", "id", "secret", true),
				Links: applink.New("https://grc.example.com", "https://one.example.com"),
			})

			err := hub.Notify(context.Background(), 9, []directorysync.Departure{{
				UserID: 1, Name: "Leaver",
				Assignments: []directorysync.Assignment{{
					Item: "A.1", Parent: "ISO 27001", Role: roleControlOwner,
					DetailURL: applink.ControlPath(7, 3),
				}},
			}})
			if err != nil {
				t.Fatalf("Notify: %v", err)
			}
			if len(sent) != 1 {
				t.Fatalf("sent %d emails, want 1", len(sent))
			}
			body := sent[0]
			for _, want := range []string{
				`href="` + tt.wantHost + `/audit/audits/7?control=3"`, // the row link
				`href="` + tt.wantHost + `/audit/dashboard"`,          // the footer button
			} {
				if !strings.Contains(body, want) {
					t.Errorf("email lacks %q:\n%s", want, body)
				}
			}
			if strings.Contains(body, tt.wantNot) {
				t.Errorf("email contains the other host %q", tt.wantNot)
			}
			if strings.Contains(body, `href="/`) {
				t.Error("email contains an unresolved relative link")
			}
		})
	}
}

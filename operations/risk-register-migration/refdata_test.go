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

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func strptr(s string) *string { return &s }

func goodRoles() []Role {
	return []Role{
		{ID: 10, RoleName: roleRiskOwner, Module: "RISK", Status: "ACTIVE"},
		{ID: 11, RoleName: roleRiskAssigner, Module: "RISK", Status: "ACTIVE"},
		{ID: 12, RoleName: roleRiskManagement, Module: "RISK", Status: "ACTIVE"},
		{ID: 99, RoleName: "grc-platform-audit-lead", Module: "AUDIT", Status: "ACTIVE"},
	}
}

func goodRefInputs() (teams []RiskTeam, cats []RiskCategory, refs []ComplianceRef, scores []RiskScore) {
	teams = []RiskTeam{
		{ID: 1, Name: "Asgardeo", Code: strptr("ASG"), Status: "ACTIVE"},
		{ID: 8, Name: "Legal", Code: nil, Status: "ACTIVE"},
	}
	cats = []RiskCategory{{ID: 3, Name: "Access Control & Credentials"}}
	refs = []ComplianceRef{{ID: 2, Name: "ISO"}, {ID: 5, Name: "SOC2"}}
	scores = []RiskScore{{ID: 7, Likelihood: 3, Impact: 3}, {ID: 4, Likelihood: 1, Impact: 2}}
	return
}

func TestBuildRefData_HappyPath(t *testing.T) {
	teams, cats, refs, scores := goodRefInputs()

	rd, err := buildRefData(teams, cats, refs, scores, goodRoles())
	if err != nil {
		t.Fatalf("buildRefData: %v", err)
	}
	if rd.TeamIDByKey["asgardeo"] != 1 || rd.TeamIDByKey["asg"] != 1 {
		t.Errorf("team keyed by name and code -> id: %+v", rd.TeamIDByKey)
	}
	if rd.TeamIDByKey["legal"] != 8 {
		t.Errorf("codeless team still keyed by name: %+v", rd.TeamIDByKey)
	}
	if rd.TeamCodeByID[1] != "ASG" || rd.TeamCodeByID[8] != "" {
		t.Errorf("TeamCodeByID: %+v", rd.TeamCodeByID)
	}
	if rd.CategoryIDByName["access control & credentials"] != 3 {
		t.Errorf("category keyed by lowercased name: %+v", rd.CategoryIDByName)
	}
	if rd.ComplianceIDByName["ISO"] != 2 || rd.ComplianceIDByName["SOC2"] != 5 {
		t.Errorf("compliance ref keyed by uppercased name: %+v", rd.ComplianceIDByName)
	}
	if rd.RoleIDByName[roleRiskOwner] != 10 || rd.RoleIDByName[roleRiskAssigner] != 11 || rd.RoleIDByName[roleRiskManagement] != 12 {
		t.Errorf("RoleIDByName: %+v", rd.RoleIDByName)
	}
}

func TestBuildRefData_Rejections(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*[]RiskTeam, *[]RiskCategory, *[]ComplianceRef, *[]RiskScore, *[]Role)
		want string
	}{
		{
			name: "empty categories",
			mut:  func(_ *[]RiskTeam, c *[]RiskCategory, _ *[]ComplianceRef, _ *[]RiskScore, _ *[]Role) { *c = nil },
			want: "reference data incomplete",
		},
		{
			name: "empty scores",
			mut:  func(_ *[]RiskTeam, _ *[]RiskCategory, _ *[]ComplianceRef, s *[]RiskScore, _ *[]Role) { *s = nil },
			want: "reference data incomplete",
		},
		{
			name: "no team has a code",
			mut: func(tm *[]RiskTeam, _ *[]RiskCategory, _ *[]ComplianceRef, _ *[]RiskScore, _ *[]Role) {
				for i := range *tm {
					(*tm)[i].Code = nil
				}
			},
			want: "no risk_team carries a code",
		},
		{
			name: "risk role missing",
			mut: func(_ *[]RiskTeam, _ *[]RiskCategory, _ *[]ComplianceRef, _ *[]RiskScore, r *[]Role) {
				*r = (*r)[1:] // drop grc-platform-risk-owner
			},
			want: `risk role "grc-platform-risk-owner" not found`,
		},
		{
			name: "risk role inactive",
			mut: func(_ *[]RiskTeam, _ *[]RiskCategory, _ *[]ComplianceRef, _ *[]RiskScore, r *[]Role) {
				(*r)[2].Status = "INACTIVE" // grc-platform-risk-management
			},
			want: `risk role "grc-platform-risk-management" has status "INACTIVE"`,
		},
		{
			name: "team key collision",
			mut: func(tm *[]RiskTeam, _ *[]RiskCategory, _ *[]ComplianceRef, _ *[]RiskScore, _ *[]Role) {
				*tm = append(*tm, RiskTeam{ID: 42, Name: "asgardeo", Code: strptr("X"), Status: "ACTIVE"})
			},
			want: "maps to both id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			teams, cats, refs, scores := goodRefInputs()
			roles := goodRoles()
			tc.mut(&teams, &cats, &refs, &scores, &roles)

			_, err := buildRefData(teams, cats, refs, scores, roles)
			if err == nil {
				t.Fatalf("want an error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// entityRefStub answers the six preflight calls with canned reference data.
func entityRefStub(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "GET /roles":
			_, _ = w.Write([]byte(`{"roles":[
				{"id":10,"roleName":"grc-platform-risk-owner","module":"RISK","status":"ACTIVE"},
				{"id":11,"roleName":"grc-platform-risk-assigner","module":"RISK","status":"ACTIVE"},
				{"id":12,"roleName":"grc-platform-risk-management","module":"RISK","status":"ACTIVE"}
			]}`))
		case "POST /risk/teams/search":
			_, _ = w.Write([]byte(`{"total":1,"teams":[{"id":1,"name":"Asgardeo","code":"ASG","status":"ACTIVE"}]}`))
		case "GET /risk/categories":
			_, _ = w.Write([]byte(`{"categories":[{"id":3,"name":"Access Control & Credentials"}]}`))
		case "POST /risk/compliance-references/search":
			_, _ = w.Write([]byte(`{"total":1,"references":[{"id":2,"name":"ISO"}]}`))
		case "GET /risk/scores":
			_, _ = w.Write([]byte(`{"scores":[{"id":7,"likelihood":3,"impact":3}]}`))
		default:
			t.Errorf("unexpected entity call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestPreflight_HappyPath(t *testing.T) {
	esrv := httptest.NewServer(entityRefStub(t))
	t.Cleanup(esrv.Close)

	stub := &scimStub{t: t, org: "wso2", total: 1, pages: map[int]string{
		1: `[{"id":"uuid-1","userName":"user1@wso2.com","name":{"givenName":"User","familyName":"One"}}]`,
	}}
	ssrv := httptest.NewServer(stub.handler())
	t.Cleanup(ssrv.Close)

	cfg := Config{
		SCIMDomain: "wso2.com",
	}
	ec := NewEntityClient(esrv.URL, 5*time.Second)
	sc := NewSCIMClient(ssrv.URL, ssrv.URL+"/t/wso2/oauth2/token", "cid", "csecret", "internal_user_mgt_list", "wso2", 5*time.Second)

	rd, users, err := preflight(context.Background(), cfg, ec, sc)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if rd.TeamIDByKey["asg"] != 1 || rd.RoleIDByName[roleRiskManagement] != 12 || rd.ComplianceIDByName["ISO"] != 2 {
		t.Fatalf("RefData not fully populated: %+v", rd)
	}
	if len(users) != 1 || users[0].Email != "user1@wso2.com" {
		t.Fatalf("preflight should return the SCIM snapshot it fetched: %+v", users)
	}
}

func TestPreflight_EmptySCIMSnapshotAborts(t *testing.T) {
	esrv := httptest.NewServer(entityRefStub(t))
	t.Cleanup(esrv.Close)

	stub := &scimStub{t: t, org: "wso2", total: 0, pages: map[int]string{1: `[]`}}
	ssrv := httptest.NewServer(stub.handler())
	t.Cleanup(ssrv.Close)

	ec := NewEntityClient(esrv.URL, 5*time.Second)
	sc := NewSCIMClient(ssrv.URL, ssrv.URL+"/t/wso2/oauth2/token", "cid", "csecret", "internal_user_mgt_list", "wso2", 5*time.Second)

	_, _, err := preflight(context.Background(), Config{SCIMDomain: "wso2.com"}, ec, sc)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("want an 'empty SCIM snapshot' error, got %v", err)
	}
}

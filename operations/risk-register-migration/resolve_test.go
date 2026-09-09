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
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

var directorySnapshot = []DirectoryUser{
	{UUID: "uuid-assigner", Email: "user1@wso2.com"},
	{UUID: "uuid-owner", Email: "user2@wso2.com"},
	{UUID: "uuid-mgr", Email: "user3@wso2.com"},
	{UUID: "uuid-action", Email: "user4@wso2.com"},
	{UUID: "uuid-dup-a", Email: "user5@wso2.com"},
	{UUID: "uuid-dup-b", Email: "user5@wso2.com"}, // same email, different uuid -> ambiguous
}

func resolvableRow(migID int, status string) Row {
	return Row{
		MigrationID:             migID,
		CSVLine:                 migID + 1,
		RiskTitle:               "R",
		AssignerEmail:           "user1@wso2.com",
		OwnerEmail:              "user2@wso2.com",
		ManagementApproverEmail: "user3@wso2.com",
		WorkflowStatus:          status,
	}
}

func TestResolverBuild_MarksAmbiguousEmail(t *testing.T) {
	r := NewResolver(nil, directorySnapshot, true)
	r.Build()

	if r.uuidByEmail["user1@wso2.com"] != "uuid-assigner" {
		t.Errorf("clean email not indexed: %v", r.uuidByEmail)
	}
	if !r.ambiguous["user5@wso2.com"] {
		t.Errorf("duplicate email not marked ambiguous: %v", r.ambiguous)
	}
	if _, ok, _ := r.userID(context.Background(), "user5@wso2.com"); ok {
		t.Errorf("ambiguous email must not resolve")
	}
}

func TestResolverApply_DryRun(t *testing.T) {
	r := NewResolver(nil, directorySnapshot, true) // nil entity: must not be called in a dry run
	r.Build()

	rows := []Row{
		resolvableRow(1, "IN_REMEDIATION"),                         // action owner missing -> REJECT
		func() Row { x := resolvableRow(2, "CLOSED"); return x }(), // action owner missing -> OK (nullable)
		func() Row {
			x := resolvableRow(3, "IN_REMEDIATION")
			x.ActionOwnerEmail = "user4@wso2.com"
			return x
		}(),
		func() Row {
			x := resolvableRow(4, "IN_REMEDIATION")
			x.ActionOwnerEmail = "user4@wso2.com"
			x.OwnerEmail = "user6@wso2.com" // not in directory -> REJECT
			return x
		}(),
		func() Row {
			x := resolvableRow(5, "CLOSED")
			x.ActionOwnerEmail = "user6@wso2.com" // not in directory, CLOSED -> WARN
			return x
		}(),
		func() Row {
			x := resolvableRow(6, "IN_REMEDIATION")
			x.ActionOwnerEmail = "user4@wso2.com"
			x.AssignerEmail = "user5@wso2.com" // ambiguous -> REJECT
			return x
		}(),
	}

	fs, err := r.Apply(context.Background(), rows)
	if err != nil {
		t.Fatalf("Apply (dry run): %v", err)
	}

	byMig := map[int][]Finding{}
	for _, f := range fs {
		byMig[f.MigrationID] = append(byMig[f.MigrationID], f)
	}

	if len(byMig[1]) != 1 || byMig[1][0].Severity != SevReject || byMig[1][0].Failure != "Action Owner" {
		t.Errorf("row 1: want one Action Owner REJECT, got %+v", byMig[1])
	}
	if len(byMig[2]) != 0 {
		t.Errorf("row 2 (CLOSED, no action owner): want no findings, got %+v", byMig[2])
	}
	if len(byMig[3]) != 0 {
		t.Errorf("row 3 (all resolvable): want no findings, got %+v", byMig[3])
	}
	if len(byMig[4]) != 1 || byMig[4][0].Severity != SevReject || byMig[4][0].Failure != "Risk Owner" {
		t.Errorf("row 4: want one Risk Owner REJECT, got %+v", byMig[4])
	}
	if !strings.Contains(byMig[4][0].Detail, "not in the SCIM directory") {
		t.Errorf("row 4 reason: %q", byMig[4][0].Detail)
	}
	if len(byMig[5]) != 1 || byMig[5][0].Severity != SevWarn || byMig[5][0].Failure != "Action Owner" {
		t.Errorf("row 5: want one Action Owner WARN, got %+v", byMig[5])
	}
	if len(byMig[6]) != 1 || byMig[6][0].Severity != SevReject || !strings.Contains(byMig[6][0].Detail, "ambiguous") {
		t.Errorf("row 6: want an 'ambiguous' Risk Assigned To REJECT, got %+v", byMig[6])
	}

	// Dry run never assigns ids.
	for _, row := range rows {
		if row.OwnerID != 0 || row.AssignerID != 0 || row.ManagementApproverID != 0 || row.ActionOwnerID != nil {
			t.Errorf("dry run assigned ids on row %d: %+v", row.MigrationID, row)
		}
	}
}

func TestResolverApply_RealRunProvisionsAndFillsIDs(t *testing.T) {
	created := map[string]bool{}
	ec := entityTestServer(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/users/by-uuid/"):
			uuid := strings.TrimPrefix(req.URL.Path, "/users/by-uuid/")
			if uuid == "uuid-owner" { // already provisioned
				_, _ = w.Write([]byte(`{"id":500}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
		case req.Method == http.MethodPost && req.URL.Path == "/users":
			var body CreateUserRequest
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body.CreatedBy != marker || body.UserType != "INTERNAL" {
				t.Errorf("CreateUser body = %+v", body)
			}
			created[body.UUID] = true
			w.WriteHeader(http.StatusCreated)
			// deterministic id per uuid for the assertion
			ids := map[string]int{"uuid-assigner": 601, "uuid-mgr": 603, "uuid-action": 604}
			_ = json.NewEncoder(w).Encode(map[string]int{"id": ids[body.UUID]})
		default:
			t.Errorf("unexpected %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	r := NewResolver(ec, directorySnapshot, false)
	r.Build()

	row := resolvableRow(1, "IN_REMEDIATION")
	row.ActionOwnerEmail = "user4@wso2.com"
	rows := []Row{row}

	fs, err := r.Apply(context.Background(), rows)
	if err != nil {
		t.Fatalf("Apply (real run): %v", err)
	}
	if len(fs) != 0 {
		t.Fatalf("want no findings, got %+v", fs)
	}
	got := rows[0]
	if got.AssignerID != 601 || got.OwnerID != 500 || got.ManagementApproverID != 603 {
		t.Errorf("ids: assigner=%d owner=%d mgr=%d", got.AssignerID, got.OwnerID, got.ManagementApproverID)
	}
	if got.ActionOwnerID == nil || *got.ActionOwnerID != 604 {
		t.Errorf("action owner id = %v", got.ActionOwnerID)
	}
	if created["uuid-owner"] {
		t.Error("must not provision an already-existing user")
	}
	for _, u := range []string{"uuid-assigner", "uuid-mgr", "uuid-action"} {
		if !created[u] {
			t.Errorf("expected %s to be provisioned", u)
		}
	}
}

func TestResolverApply_RealRunTransportErrorAborts(t *testing.T) {
	ec := entityTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	r := NewResolver(ec, directorySnapshot, false)
	r.Build()

	_, err := r.Apply(context.Background(), []Row{resolvableRow(1, "CLOSED")})
	if err == nil {
		t.Fatal("want an error from a 502 during resolution, got nil")
	}
}

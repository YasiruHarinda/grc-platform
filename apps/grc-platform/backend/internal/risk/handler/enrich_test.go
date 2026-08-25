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
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
)

// dirServer is a stand-in identity directory that counts how many searches it
// is asked to perform — the number this whole batching exercise exists to keep
// down.
type dirServer struct {
	srv    *httptest.Server
	calls  atomic.Int32
	people map[string]string // uuid -> display name
}

func newDirServer(t *testing.T, people map[string]string) *dirServer {
	t.Helper()
	d := &dirServer{people: people}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("POST /t/wso2/scim2/Users/.search", func(w http.ResponseWriter, r *http.Request) {
		d.calls.Add(1)
		var body struct {
			Filter string `json:"filter"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		resources := []map[string]any{}
		for uuid, name := range d.people {
			// The client sends `id eq "<uuid>"`; a substring match is enough to
			// pick the right person here.
			if len(body.Filter) > 0 && contains(body.Filter, uuid) {
				resources = append(resources, map[string]any{
					"id": uuid, "userName": uuid + "@wso2.com",
					"name": map[string]any{"givenName": name, "familyName": ""},
				})
			}
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Resources": resources})
	})

	d.srv = httptest.NewServer(mux)
	t.Cleanup(d.srv.Close)
	return d
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func (d *dirServer) deps() *Deps {
	c := scim.NewClient(d.srv.URL, d.srv.URL+"/oauth2/token", "id", "secret", "scope", "wso2")
	return &Deps{Directory: directory.New(c, time.Hour)}
}

func TestEnrichListItems_ResolvesNamesInOneBatchPerPerson(t *testing.T) {
	const ownerUUID, assignerUUID = "owner-uuid", "assigner-uuid"
	srv := newDirServer(t, map[string]string{
		ownerUUID:    "Directory Owner",
		assignerUUID: "Directory Assigner",
	})
	d := srv.deps()

	// The same two people across 25 risks — the realistic shape of a register
	// page, and the case where per-row resolution would be worst.
	items := make([]*model.RiskListItem, 25)
	for i := range items {
		items[i] = &model.RiskListItem{
			OwnerUUID: ownerUUID, OwnerName: "Stale Owner",
			AssignerUUID: assignerUUID, AssignerName: "Stale Assigner",
		}
	}

	d.enrichListItems(context.Background(), items)

	for i, it := range items {
		if it.OwnerName != "Directory Owner" || it.AssignerName != "Directory Assigner" {
			t.Fatalf("item %d not enriched: owner=%q assigner=%q", i, it.OwnerName, it.AssignerName)
		}
	}

	// The actual claim. 25 rows naming 2 distinct people is 50 uuids; resolving
	// per row would be 50 calls to a service behind an API gateway. Anything
	// above 2 means the de-duplication or the cache stopped working — which
	// would not fail any assertion above, since the names would still be right.
	if got := srv.calls.Load(); got != 2 {
		t.Errorf("directory searched %d times for 25 rows naming 2 people, want 2", got)
	}
}

// The behaviour that lets this ship before the directory is reachable
// everywhere: a name the data layer already supplied is never replaced with
// nothing.
func TestEnrichListItems_KeepsStoredNameWhenDirectoryHasNoAnswer(t *testing.T) {
	d := newDirServer(t, map[string]string{}).deps()

	items := []*model.RiskListItem{{
		OwnerUUID: "unknown-uuid", OwnerName: "Stored Owner",
		AssignerUUID: "", AssignerName: "Stored Assigner",
	}}
	d.enrichListItems(context.Background(), items)

	if items[0].OwnerName != "Stored Owner" {
		t.Errorf("owner name was clobbered: %q", items[0].OwnerName)
	}
	if items[0].AssignerName != "Stored Assigner" {
		t.Errorf("assigner name was clobbered: %q", items[0].AssignerName)
	}
}

func TestEnrichListItems_NoDirectoryConfiguredIsANoOp(t *testing.T) {
	d := &Deps{Directory: nil}
	items := []*model.RiskListItem{{OwnerUUID: "u", OwnerName: "Stored"}}

	d.enrichListItems(context.Background(), items) // must not panic

	if items[0].OwnerName != "Stored" {
		t.Errorf("name changed with no directory: %q", items[0].OwnerName)
	}
}

func TestEnrichDetail_ResolvesAllThreeRoles(t *testing.T) {
	d := newDirServer(t, map[string]string{
		"o": "Owner Person", "a": "Assigner Person", "m": "Approver Person",
	}).deps()

	detail := &model.RiskDetail{
		OwnerUUID: "o", OwnerName: "old",
		AssignerUUID: "a", AssignerName: "old",
		ManagementApproverUUID: "m", ManagementApproverName: "old",
	}
	d.enrichDetail(context.Background(), detail)

	if detail.OwnerName != "Owner Person" ||
		detail.AssignerName != "Assigner Person" ||
		detail.ManagementApproverName != "Approver Person" {
		t.Errorf("detail not fully enriched: %+v", detail)
	}
}

func TestEnrichDetail_NilDetailIsANoOp(t *testing.T) {
	d := newDirServer(t, nil).deps()
	d.enrichDetail(context.Background(), nil) // must not panic
}

func TestEnrichDashboard_ResolvesHighRiskOwners(t *testing.T) {
	srv := newDirServer(t, map[string]string{"o1": "Owner One", "o2": "Owner Two"})
	d := srv.deps()

	dash := &model.DashboardSummary{HighRisks: []model.HighRiskItem{
		{OwnerUUID: "o1", OwnerName: "stale"},
		{OwnerUUID: "o2", OwnerName: "stale"},
		{OwnerUUID: "o1", OwnerName: "stale"}, // repeat: must not cost a second call
		{OwnerUUID: "", OwnerName: "Not Backfilled"},
	}}
	d.enrichDashboard(context.Background(), dash)

	if dash.HighRisks[0].OwnerName != "Owner One" || dash.HighRisks[2].OwnerName != "Owner One" {
		t.Errorf("o1 rows not enriched: %+v", dash.HighRisks)
	}
	if dash.HighRisks[1].OwnerName != "Owner Two" {
		t.Errorf("o2 row not enriched: %q", dash.HighRisks[1].OwnerName)
	}
	if dash.HighRisks[3].OwnerName != "Not Backfilled" {
		t.Errorf("a row with no uuid must keep its stored name, got %q", dash.HighRisks[3].OwnerName)
	}
	if got := srv.calls.Load(); got != 2 {
		t.Errorf("directory searched %d times for 2 distinct owners, want 2", got)
	}
}

func TestEnrichAnalytics_ResolvesAgingRiskOwners(t *testing.T) {
	d := newDirServer(t, map[string]string{"a1": "Aging Owner"}).deps()

	sum := &model.AnalyticsSummary{AgingRisks: []model.AgingRiskItem{
		{OwnerUUID: "a1", OwnerName: "stale"},
		{OwnerUUID: "unknown", OwnerName: "Stored Name"},
	}}
	d.enrichAnalytics(context.Background(), sum)

	if sum.AgingRisks[0].OwnerName != "Aging Owner" {
		t.Errorf("not enriched: %q", sum.AgingRisks[0].OwnerName)
	}
	if sum.AgingRisks[1].OwnerName != "Stored Name" {
		t.Errorf("unknown uuid must keep the stored name, got %q", sum.AgingRisks[1].OwnerName)
	}
}

func TestEnrichDashboardAndAnalytics_NilInputsAreNoOps(t *testing.T) {
	d := newDirServer(t, nil).deps()
	// Both must tolerate nil and empty payloads — the handlers have an
	// early-return path that writes an empty summary.
	d.enrichDashboard(context.Background(), nil)
	d.enrichAnalytics(context.Background(), nil)
	d.enrichDashboard(context.Background(), &model.DashboardSummary{})
	d.enrichAnalytics(context.Background(), &model.AnalyticsSummary{})
}

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

package directory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
)

const testUUID = "885aeeb0-2086-4ca4-83c9-b2a62b299967"
const testExternalUUID = "f30c1c9a-6c1e-4c7c-9d9a-6a5f8e6a2222"

// fakeDirectory stands in for the SCIM Operations Service. Serving it over real
// HTTP keeps the scim.Client's own request building, status handling and JSON
// decoding in the test rather than stubbing them out — those are the parts most
// likely to be wrong.
type fakeDirectory struct {
	srv *httptest.Server
	// calls counts internal-org user searches, so a test can prove the cache
	// prevented one.
	calls atomic.Int32
	// down makes every call fail, simulating an unreachable directory.
	down atomic.Bool
	// known is whether the internal-org user search finds anybody.
	known atomic.Bool
	name  atomic.Value // string

	// externalCalls/externalKnown are the external-org equivalents of
	// calls/known — the two orgs are genuinely separate identity spaces (see
	// LookupTyped), so each gets its own knowledge of who exists.
	externalCalls atomic.Int32
	externalKnown atomic.Bool
	externalName  atomic.Value // string
}

func newFakeDirectory(t *testing.T) *fakeDirectory {
	t.Helper()
	f := &fakeDirectory{}
	f.known.Store(true)
	f.name.Store("Nimali Perera")
	f.externalKnown.Store(true)
	f.externalName.Store("Alex External")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if f.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("POST /t/wso2/scim2/Users/.search", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if f.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var in struct {
			Filter string `json:"filter"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		resources := []map[string]any{}
		// Only matches testUUID's own filter — a query for any other uuid
		// (including one that's real in the external org) finds nobody here,
		// same as a real Asgardeo org search would for an id it doesn't hold.
		if f.known.Load() && in.Filter == fmt.Sprintf("id eq %q", testUUID) {
			parts := map[string]any{"givenName": f.name.Load().(string), "familyName": ""}
			resources = append(resources, map[string]any{
				"id": testUUID, "userName": "nimali.re@wso2.com", "name": parts,
			})
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Resources": resources})
	})
	mux.HandleFunc("POST /t/wso2external/scim2/Users/.search", func(w http.ResponseWriter, r *http.Request) {
		f.externalCalls.Add(1)
		if f.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var in struct {
			Filter string `json:"filter"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		resources := []map[string]any{}
		if f.externalKnown.Load() && in.Filter == fmt.Sprintf("id eq %q", testExternalUUID) {
			parts := map[string]any{"givenName": f.externalName.Load().(string), "familyName": ""}
			resources = append(resources, map[string]any{
				"id": testExternalUUID, "userName": "alex.external@partner.example", "name": parts,
			})
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Resources": resources})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeDirectory) service(ttl time.Duration) *directory.Service {
	c := scim.NewClient(f.srv.URL, f.srv.URL+"/oauth2/token", "id", "secret", "scope", "wso2")
	return directory.New(c, ttl)
}

// serviceWithExternal is service plus an external-org client pointed at the
// same fake server's external-org path, for LookupTyped/LookupAllTyped tests.
// Both the internal-fallback and external caches share ttl here — tests using
// this helper assert call counts within a single TTL window, not expiry, so a
// single knob is enough.
func (f *fakeDirectory) serviceWithExternal(ttl time.Duration) *directory.Service {
	c := scim.NewClient(f.srv.URL, f.srv.URL+"/oauth2/token", "id", "secret", "scope", "wso2")
	ec := scim.NewExternalClient(f.srv.URL, f.srv.URL+"/oauth2/token", "id", "secret", "scope", "wso2external")
	return directory.NewWithExternal(c, ec, ttl, ttl)
}

func TestLookup_ResolvesAndCaches(t *testing.T) {
	f := newFakeDirectory(t)
	svc := f.service(time.Hour)

	for i := 0; i < 3; i++ {
		p, ok := svc.Lookup(context.Background(), testUUID)
		if !ok {
			t.Fatalf("lookup %d: not resolved", i)
		}
		if p.DisplayName != "Nimali Perera" || p.Email != "nimali.re@wso2.com" {
			t.Fatalf("lookup %d: got %+v", i, p)
		}
	}
	if got := f.calls.Load(); got != 1 {
		t.Errorf("directory called %d times for 3 lookups, want 1 — the cache did not hold", got)
	}
}

// The reason this cache exists. Notification recipients resolve through here,
// and sendRiskEvent drops a recipient with no address then refuses to send with
// none left — so returning nothing on a directory blip would silently swallow
// escalation notices. A stale address is far likelier to be right than absent.
func TestLookup_ServesStaleWhenDirectoryIsDown(t *testing.T) {
	f := newFakeDirectory(t)
	// A TTL that has already elapsed by the second call, so the refresh path
	// runs rather than the cache-hit path.
	svc := f.service(time.Millisecond)

	if _, ok := svc.Lookup(context.Background(), testUUID); !ok {
		t.Fatal("first lookup should resolve")
	}
	time.Sleep(5 * time.Millisecond)

	f.down.Store(true)
	p, ok := svc.Lookup(context.Background(), testUUID)
	if !ok {
		t.Fatal("SECURITY/DELIVERY: a stale value must be served when the directory is unreachable")
	}
	if p.Email != "nimali.re@wso2.com" {
		t.Errorf("stale value lost its email: %+v", p)
	}
}

func TestLookup_UnreachableWithNothingCachedIsUnresolved(t *testing.T) {
	f := newFakeDirectory(t)
	f.down.Store(true)
	svc := f.service(time.Hour)

	if _, ok := svc.Lookup(context.Background(), testUUID); ok {
		t.Error("nothing cached and the directory down must not resolve")
	}
}

// A negative answer is cached so an unresolvable uuid does not re-ask on every
// render — but it is not served stale, so someone who has just been given an
// account becomes visible once the TTL passes instead of staying invisible for
// the life of the process.
func TestLookup_NegativeIsCachedButRetriedAfterTTL(t *testing.T) {
	f := newFakeDirectory(t)
	f.known.Store(false)
	svc := f.service(50 * time.Millisecond)

	if _, ok := svc.Lookup(context.Background(), testUUID); ok {
		t.Fatal("unknown uuid must not resolve")
	}
	if _, ok := svc.Lookup(context.Background(), testUUID); ok {
		t.Fatal("still must not resolve")
	}
	if got := f.calls.Load(); got != 1 {
		t.Errorf("directory called %d times, want 1 — the negative was not cached", got)
	}

	// The account now exists.
	f.known.Store(true)
	time.Sleep(60 * time.Millisecond)
	if _, ok := svc.Lookup(context.Background(), testUUID); !ok {
		t.Error("a negative answer must be re-asked once its TTL passes")
	}
}

func TestLookup_EmptyUUIDNeverReachesTheDirectory(t *testing.T) {
	f := newFakeDirectory(t)
	svc := f.service(time.Hour)

	if _, ok := svc.Lookup(context.Background(), ""); ok {
		t.Error("an empty uuid must not resolve")
	}
	if got := f.calls.Load(); got != 0 {
		t.Errorf("directory called %d times for an empty uuid, want 0", got)
	}
}

// A nil client is local development with no credentials for this internal
// service. Lookups must answer "unknown" rather than panicking.
func TestLookup_NilClientIsUnresolvedNotAPanic(t *testing.T) {
	svc := directory.New(nil, time.Hour)
	if _, ok := svc.Lookup(context.Background(), testUUID); ok {
		t.Error("a nil client must not resolve anybody")
	}
}

func TestLookupAll_DeduplicatesAndOmitsUnknown(t *testing.T) {
	f := newFakeDirectory(t)
	svc := f.service(time.Hour)

	got := svc.LookupAll(context.Background(), []string{testUUID, testUUID, "", testUUID})
	if len(got) != 1 {
		t.Fatalf("got %d people, want 1: %+v", len(got), got)
	}
	if got[testUUID].Email != "nimali.re@wso2.com" {
		t.Errorf("wrong person: %+v", got[testUUID])
	}
	if calls := f.calls.Load(); calls != 1 {
		t.Errorf("directory called %d times for a repeated uuid, want 1", calls)
	}

	f.known.Store(false)
	if out := svc.LookupAll(context.Background(), []string{"nobody-at-all"}); len(out) != 0 {
		t.Errorf("unknown uuids must be omitted, got %+v", out)
	}
}

// TestLookupTyped_RoutesInternalThroughExistingPath confirms an INTERNAL
// user_type resolves exactly as plain Lookup does (bulk/per-uuid internal
// path), never touching the external-org endpoint.
func TestLookupTyped_RoutesInternalThroughExistingPath(t *testing.T) {
	f := newFakeDirectory(t)
	svc := f.serviceWithExternal(time.Hour)

	p, ok := svc.LookupTyped(context.Background(), testUUID, "INTERNAL")
	if !ok || p.DisplayName != "Nimali Perera" {
		t.Fatalf("got (%+v, %v), want the internal-org user resolved", p, ok)
	}
	if got := f.externalCalls.Load(); got != 0 {
		t.Errorf("external-org endpoint called %d times for an INTERNAL lookup, want 0", got)
	}
}

// TestLookupTyped_RoutesExternalThroughExternalClient confirms an EXTERNAL
// user_type is resolved via the external-org client, cached the same way the
// internal per-uuid fallback is.
func TestLookupTyped_RoutesExternalThroughExternalClient(t *testing.T) {
	f := newFakeDirectory(t)
	svc := f.serviceWithExternal(time.Hour)

	for i := 0; i < 3; i++ {
		p, ok := svc.LookupTyped(context.Background(), testExternalUUID, "EXTERNAL")
		if !ok || p.DisplayName != "Alex External" {
			t.Fatalf("lookup %d: got (%+v, %v), want the external-org user resolved", i, p, ok)
		}
	}
	if got := f.externalCalls.Load(); got != 1 {
		t.Errorf("external-org endpoint called %d times for 3 lookups, want 1 — the cache did not hold", got)
	}
	if got := f.calls.Load(); got != 0 {
		t.Errorf("internal-org endpoint called %d times for an EXTERNAL lookup, want 0", got)
	}
}

// TestLookupTyped_CrossOrgUUIDFailsRatherThanResolving is the design doc's
// (§8) explicit assertion: the two SCIM orgs are separate identity spaces, so
// asking for a known-internal uuid as EXTERNAL (or vice versa) must fail the
// lookup, not silently resolve — cross-wiring a user_type would otherwise be
// invisible.
func TestLookupTyped_CrossOrgUUIDFailsRatherThanResolving(t *testing.T) {
	f := newFakeDirectory(t)
	svc := f.serviceWithExternal(time.Hour)

	if _, ok := svc.LookupTyped(context.Background(), testUUID, "EXTERNAL"); ok {
		t.Error("an internal-only uuid looked up as EXTERNAL must not resolve")
	}
	if _, ok := svc.LookupTyped(context.Background(), testExternalUUID, "INTERNAL"); ok {
		t.Error("an external-only uuid looked up as INTERNAL must not resolve")
	}
}

// TestLookupTyped_NilExternalClientIsUnresolvedNotAPanic mirrors
// TestLookup_NilClientIsUnresolvedNotAPanic for the external path — local
// development without external-org credentials must degrade, not crash.
func TestLookupTyped_NilExternalClientIsUnresolvedNotAPanic(t *testing.T) {
	svc := directory.NewWithExternal(nil, nil, time.Hour, time.Hour)
	if _, ok := svc.LookupTyped(context.Background(), testExternalUUID, "EXTERNAL"); ok {
		t.Error("a nil external client must not resolve anybody")
	}
}

// TestLookupAllTyped_RoutesEachUUIDByItsOwnType confirms LookupAllTyped
// dispatches each uuid through LookupTyped individually rather than assuming
// one type for the whole batch.
func TestLookupAllTyped_RoutesEachUUIDByItsOwnType(t *testing.T) {
	f := newFakeDirectory(t)
	svc := f.serviceWithExternal(time.Hour)

	got := svc.LookupAllTyped(context.Background(), map[string]string{
		testUUID:         "INTERNAL",
		testExternalUUID: "EXTERNAL",
		"":               "EXTERNAL",
	})
	if len(got) != 2 {
		t.Fatalf("got %d people, want 2: %+v", len(got), got)
	}
	if got[testUUID].DisplayName != "Nimali Perera" {
		t.Errorf("wrong internal person: %+v", got[testUUID])
	}
	if got[testExternalUUID].DisplayName != "Alex External" {
		t.Errorf("wrong external person: %+v", got[testExternalUUID])
	}
}

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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/directory"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
)

const testUUID = "885aeeb0-2086-4ca4-83c9-b2a62b299967"

// fakeDirectory stands in for the SCIM Operations Service. Serving it over real
// HTTP keeps the scim.Client's own request building, status handling and JSON
// decoding in the test rather than stubbing them out — those are the parts most
// likely to be wrong.
type fakeDirectory struct {
	srv *httptest.Server
	// calls counts user searches, so a test can prove the cache prevented one.
	calls atomic.Int32
	// down makes every call fail, simulating an unreachable directory.
	down atomic.Bool
	// known is whether the user search finds anybody.
	known atomic.Bool
	name  atomic.Value // string
}

func newFakeDirectory(t *testing.T) *fakeDirectory {
	t.Helper()
	f := &fakeDirectory{}
	f.known.Store(true)
	f.name.Store("Nimali Perera")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if f.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	})
	mux.HandleFunc("POST /organizations/internal/users/search", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if f.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		resources := []map[string]any{}
		if f.known.Load() {
			parts := map[string]any{"givenName": f.name.Load().(string), "familyName": ""}
			resources = append(resources, map[string]any{
				"id": testUUID, "userName": "nimali.re@wso2.com", "name": parts,
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
	c := scim.NewClient(f.srv.URL, f.srv.URL+"/oauth2/token", "id", "secret", "scope")
	return directory.New(c, ttl)
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

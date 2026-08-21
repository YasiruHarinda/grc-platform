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

// Package directory answers "who is this uuid" — a person's name and email —
// from the identity provider, with a cache in front of it.
//
// A security review required that this platform stop storing user names and
// emails. Once it does, every place that shows a person's name has to ask
// somewhere else, and that somewhere is a remote service behind an API
// gateway. This package is what keeps that from turning one page render into
// dozens of network calls, or one directory outage into an outage here.
package directory

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/scim"
)

// DefaultTTL is how long a resolved person is reused before being refreshed.
//
// Names and email addresses change on the order of never, so this trades
// freshness that nobody perceives for a large reduction in calls to a service
// that is not on this platform's side of the network. Governs only the
// per-uuid fallback path — see StartBulkRefresh for the primary one.
const DefaultTTL = time.Hour

// DefaultBulkRefreshInterval is how often StartBulkRefresh re-fetches the
// whole directory snapshot.
const DefaultBulkRefreshInterval = 12 * time.Hour

// Person is a resolved identity.
type Person struct {
	UUID        string
	Email       string
	DisplayName string
}

type entry struct {
	person Person
	// found distinguishes "the directory says no such user" from "not looked up
	// yet". A negative answer is cached too — an unresolvable uuid is usually
	// permanent (a deleted account), and without this every render of the risk
	// naming them would ask again.
	found bool
	// refreshAt is when the entry stops being served without a refresh attempt.
	// It is NOT when the entry is discarded: see Lookup.
	refreshAt time.Time
}

// Service resolves uuids to people, caching what it learns.
type Service struct {
	scim *scim.Client
	ttl  time.Duration

	mu    sync.RWMutex
	items map[string]entry

	bulkMu sync.RWMutex
	bulk   map[string]Person
}

// New returns a Service backed by client. A nil client is allowed and makes
// every lookup unresolved — local development without credentials for this
// internal service, where showing no name is preferable to failing.
func New(client *scim.Client, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Service{scim: client, ttl: ttl, items: make(map[string]entry)}
}

// Lookup resolves a uuid to a person, reporting whether the directory knows
// them.
//
// # Stale entries are served when the directory cannot be reached
//
// The TTL governs when an entry is REFRESHED, not when it is discarded. If a
// refresh fails, the previous value is returned rather than an error.
//
// This matters most for email. Notification recipients are resolved through
// here, and sendRiskEvent fails closed — a recipient with no address is
// dropped, and a send with no recipients does not happen. Without stale
// serving, a directory blip would silently swallow escalation notices, which
// are the ones people actually depend on. An address that is an hour out of
// date is almost certainly still correct; no address at all is certainly wrong.
//
// A negative answer is not served stale: it is re-asked once its TTL passes, so
// a person who has just been given an account resolves rather than staying
// invisible for as long as the process lives.
func (s *Service) Lookup(ctx context.Context, uuid string) (Person, bool) {
	if uuid == "" {
		return Person{}, false
	}

	if p, ok := s.bulkLookup(uuid); ok {
		return p, true
	}

	s.mu.RLock()
	cached, cachedOK := s.items[uuid]
	s.mu.RUnlock()
	if cachedOK && time.Now().Before(cached.refreshAt) {
		return cached.person, cached.found
	}

	dirUser, err := s.scim.LookupByUUID(ctx, uuid)
	if err != nil {
		// Serve stale rather than nothing — but only a positive answer. A stale
		// "unknown" is not worth preserving; the next call can ask again.
		if cachedOK && cached.found {
			slog.WarnContext(ctx, "directory: lookup failed, serving the last known value",
				"ageLimit", s.ttl, "err", err)
			return cached.person, true
		}
		slog.WarnContext(ctx, "directory: lookup failed and nothing cached", "err", err)
		return Person{}, false
	}

	e := entry{refreshAt: time.Now().Add(s.ttl)}
	if dirUser != nil {
		e.person = Person{UUID: dirUser.UUID, Email: dirUser.Email, DisplayName: dirUser.DisplayName}
		e.found = true
	}
	s.mu.Lock()
	s.items[uuid] = e
	s.mu.Unlock()
	return e.person, e.found
}

// LookupAll resolves several uuids at once, de-duplicating them.
//
// Currently a loop over Lookup, so cached uuids cost nothing and only the
// misses reach the directory. Callers should use this rather than looping
// themselves, so that batching the misses into a single directory call later is
// a change here and nowhere else.
//
// The returned map omits uuids the directory does not know, so a caller ranging
// over it sees only people it can actually name.
func (s *Service) LookupAll(ctx context.Context, uuids []string) map[string]Person {
	out := make(map[string]Person, len(uuids))
	for _, u := range uuids {
		if u == "" {
			continue
		}
		if _, done := out[u]; done {
			continue
		}
		if p, ok := s.Lookup(ctx, u); ok {
			out[u] = p
		}
	}
	return out
}

func (s *Service) bulkLookup(uuid string) (Person, bool) {
	s.bulkMu.RLock()
	defer s.bulkMu.RUnlock()
	p, ok := s.bulk[uuid]
	return p, ok
}

// SearchDomain returns every bulk-snapshot person whose name or email
// contains query, case-insensitively. Powers the Admin Console's "Add User"
// typeahead (see ADMIN_CONSOLE_DESIGN.md §5.1) — a substring match over the
// snapshot StartBulkRefresh already keeps warm, so this costs no directory
// call and cannot lag by more than one refresh interval. Deliberately not a
// live SCIM search: this is an admin-only, low-frequency lookup, not worth a
// new dependency on the request path.
//
// An empty query returns nothing rather than the whole snapshot — the caller
// is expected to enforce a minimum length before calling this, but refusing
// "" here too means that requirement can never be silently skipped.
//
// Results are unordered; callers needing a stable order should sort.
func (s *Service) SearchDomain(query string) []Person {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []Person{}
	}

	s.bulkMu.RLock()
	defer s.bulkMu.RUnlock()

	out := make([]Person, 0, 8)
	for _, p := range s.bulk {
		if strings.Contains(strings.ToLower(p.DisplayName), query) || strings.Contains(strings.ToLower(p.Email), query) {
			out = append(out, p)
		}
	}
	return out
}

// StartBulkRefresh fetches every directory user whose email is in domain (see
// scim.Client.ListUsersByDomain) and keeps that snapshot current in the
// background, replacing it every interval. Lookup and LookupAll check this
// snapshot before falling back to their per-uuid path, so the great majority
// of resolutions — anyone in the domain — cost no directory call at all
// rather than one per uuid.
//
// interval <= 0 uses DefaultBulkRefreshInterval.
//
// The first fetch happens synchronously so the snapshot is warm as soon as
// this returns, but its failure is not fatal: it is logged and the service
// falls back to the per-uuid path (as it would with no bulk cache at all)
// until the next scheduled attempt succeeds. Resolving a name is a display
// nicety, not something worth failing startup over.
//
// The background refresh loop runs until ctx is done.
func (s *Service) StartBulkRefresh(ctx context.Context, domain string, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultBulkRefreshInterval
	}

	s.refreshBulk(ctx, domain)

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.refreshBulk(ctx, domain)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Service) refreshBulk(ctx context.Context, domain string) {
	if s.scim == nil {
		return
	}
	users, err := s.scim.ListUsersByDomain(ctx, domain)
	if err != nil {
		// Leave the previous snapshot in place — stale is better than empty,
		// and Lookup already falls back to the per-uuid path for anyone this
		// snapshot doesn't (yet, or ever) know about.
		slog.WarnContext(ctx, "directory: bulk refresh failed, keeping the last known snapshot",
			"domain", domain, "err", err)
		return
	}

	next := make(map[string]Person, len(users))
	for _, u := range users {
		next[u.UUID] = Person{UUID: u.UUID, Email: u.Email, DisplayName: u.DisplayName}
	}

	s.bulkMu.Lock()
	s.bulk = next
	s.bulkMu.Unlock()
	slog.InfoContext(ctx, "directory: bulk snapshot refreshed", "domain", domain, "count", len(next))
}

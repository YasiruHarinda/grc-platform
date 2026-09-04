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

// DefaultBulkRefreshHourUTC is the wall-clock hour (UTC, 0–23) StartBulkRefresh
// re-fetches the whole snapshot at each day. A fixed off-hours time keeps the
// burst of paging calls predictable regardless of when a deploy restarts.
const DefaultBulkRefreshHourUTC = 1

// bulkRefreshRetryInterval is how soon StartBulkRefresh retries after a failed
// fetch instead of waiting for the next daily slot. A failure at startup is the
// likely one — the directory is remote and can cold-start slowly — and until it
// succeeds SearchDomain has an empty snapshot and no per-uuid path to fall back
// on, so a whole day of waiting would silently break the Add User typeahead.
const bulkRefreshRetryInterval = 15 * time.Minute

// DefaultExternalTTL is how long a resolved external-org person is reused
// before being refreshed (see lookupExternal). Longer than DefaultTTL: the
// external org has no bulk snapshot to fall back on, so this per-uuid entry
// is the only cache external lookups get, and names/emails there are exactly
// as stable as internal ones — no reason to re-ask more often just because
// this path happens to be per-uuid instead of bulk.
const DefaultExternalTTL = 24 * time.Hour

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
	// externalSCIM resolves `user_type = EXTERNAL` identities, which live in a
	// genuinely separate Asgardeo organization from scim — see LookupTyped.
	// nil unless set via NewWithExternal, same nil-tolerance as scim itself:
	// every external lookup then answers "unknown" rather than panicking.
	externalSCIM *scim.Client
	ttl          time.Duration
	// externalTTL governs the external per-uuid cache (see lookupExternal).
	// See DefaultExternalTTL for why it defaults higher than ttl.
	externalTTL time.Duration

	mu    sync.RWMutex
	items map[string]entry

	bulkMu sync.RWMutex
	bulk   map[string]Person
}

// New returns a Service backed by client. A nil client is allowed and makes
// every lookup unresolved — local development without credentials for this
// internal service, where showing no name is preferable to failing.
//
// Lookup/LookupAll built on this Service only ever resolve against client's
// org (in practice always the internal one — every caller today is Risk,
// which has no external users). Audit, which does, uses NewWithExternal and
// LookupTyped/LookupAllTyped instead.
func New(client *scim.Client, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Service{scim: client, ttl: ttl, items: make(map[string]entry)}
}

// NewWithExternal is New plus a second client for the external Asgardeo org,
// enabling LookupTyped/LookupAllTyped's EXTERNAL routing. externalClient may
// be nil (local development without credentials for that org), in which case
// every external lookup answers "unknown" — the same degrade-rather-than-fail
// contract client's own nil-tolerance already gives internal lookups.
//
// externalTTL governs the external per-uuid cache; externalTTL <= 0 uses
// DefaultExternalTTL, not DefaultTTL — see the field comment on why.
func NewWithExternal(client, externalClient *scim.Client, ttl, externalTTL time.Duration) *Service {
	s := New(client, ttl)
	s.externalSCIM = externalClient
	if externalTTL <= 0 {
		externalTTL = DefaultExternalTTL
	}
	s.externalTTL = externalTTL
	return s
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

// LookupTyped is Lookup with an explicit routing hint: userType is the
// caller's local user.user_type value ("INTERNAL" or "EXTERNAL"). It exists
// because directory.Service has no database access of its own — it cannot
// tell an internal uuid from an external one by the string alone, so a caller
// that has both org types (Audit) must say which this uuid is.
//
// Any userType other than the literal "EXTERNAL" is treated as INTERNAL,
// matching the user.user_type column's own NOT NULL DEFAULT 'INTERNAL'. An
// INTERNAL uuid resolves exactly as Lookup already does (bulk snapshot, then
// the per-uuid internal fallback). An EXTERNAL uuid skips the bulk snapshot
// (internal-org only) and goes straight to a per-uuid call against
// externalSCIM, cached the same way. A uuid resolved as one type is never
// findable as the other — the two SCIM orgs don't share identities — which is
// deliberate: cross-wiring a uuid to the wrong org should fail the lookup,
// not silently resolve to nothing or, worse, someone else.
func (s *Service) LookupTyped(ctx context.Context, uuid, userType string) (Person, bool) {
	if userType != "EXTERNAL" {
		return s.Lookup(ctx, uuid)
	}
	return s.lookupExternal(ctx, uuid)
}

// LookupAllTyped is LookupAll for callers that need per-uuid EXTERNAL/INTERNAL
// routing (see LookupTyped) — uuidTypes maps a uuid to its local
// user.user_type. An empty-string uuid key is skipped, same as LookupAll.
func (s *Service) LookupAllTyped(ctx context.Context, uuidTypes map[string]string) map[string]Person {
	out := make(map[string]Person, len(uuidTypes))
	for uuid, userType := range uuidTypes {
		if uuid == "" {
			continue
		}
		if p, ok := s.LookupTyped(ctx, uuid, userType); ok {
			out[uuid] = p
		}
	}
	return out
}

// lookupExternal is Lookup's per-uuid-only path (no bulk snapshot — the
// external org has no equivalent of ListUsersByDomain to build one from)
// against externalSCIM, cached in the same items map as the internal fallback
// path under a distinct key so an internal and external uuid that happened to
// collide as strings could never share a cache entry.
func (s *Service) lookupExternal(ctx context.Context, uuid string) (Person, bool) {
	if uuid == "" {
		return Person{}, false
	}
	key := "external:" + uuid

	s.mu.RLock()
	cached, cachedOK := s.items[key]
	s.mu.RUnlock()
	if cachedOK && time.Now().Before(cached.refreshAt) {
		return cached.person, cached.found
	}

	dirUser, err := s.externalSCIM.LookupByUUID(ctx, uuid)
	if err != nil {
		if cachedOK && cached.found {
			slog.WarnContext(ctx, "directory: external lookup failed, serving the last known value",
				"ageLimit", s.externalTTL, "err", err)
			return cached.person, true
		}
		slog.WarnContext(ctx, "directory: external lookup failed and nothing cached", "err", err)
		return Person{}, false
	}

	e := entry{refreshAt: time.Now().Add(s.externalTTL)}
	if dirUser != nil {
		e.person = Person{UUID: dirUser.UUID, Email: dirUser.Email, DisplayName: dirUser.DisplayName}
		e.found = true
	}
	s.mu.Lock()
	s.items[key] = e
	s.mu.Unlock()
	return e.person, e.found
}

func (s *Service) bulkLookup(uuid string) (Person, bool) {
	s.bulkMu.RLock()
	defer s.bulkMu.RUnlock()
	p, ok := s.bulk[uuid]
	return p, ok
}

// SearchDomain returns every bulk-snapshot person whose name or email
// contains query, case-insensitively. Powers the Admin Console's "Add User"
// typeahead — a substring match over the snapshot StartBulkRefresh already
// keeps warm, so this costs no directory call and cannot lag by more than one
// daily refresh. Deliberately not a live SCIM search: this is an admin-only,
// low-frequency lookup, not worth a new dependency on the request path.
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

// SearchExternal is SearchDomain's counterpart for the external Asgardeo org —
// the Add User dialog's External typeahead. Unlike SearchDomain, this is a
// live SCIM call (scim.Client.SearchByQuery), not a snapshot match: the
// external org has no bulk-fetch equivalent to keep warm (see
// NewWithExternal). Returns nil when externalSCIM is unset (local dev without
// credentials for that org), the same degrade-rather-than-fail contract as
// every other external lookup here.
func (s *Service) SearchExternal(ctx context.Context, query string) ([]Person, error) {
	if s.externalSCIM == nil {
		return nil, nil
	}
	users, err := s.externalSCIM.SearchByQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]Person, 0, len(users))
	for _, u := range users {
		out = append(out, Person{UUID: u.UUID, Email: u.Email, DisplayName: u.DisplayName})
	}
	return out, nil
}

// StartBulkRefresh fetches every directory user whose email is in domain (see
// scim.Client.ListUsersByDomain) and keeps that snapshot warm for Lookup /
// LookupAll: once immediately, then daily at hourUTC:00 UTC (out-of-range
// hourUTC uses DefaultBulkRefreshHourUTC), retrying sooner after a failure.
// A failed fetch is logged, not fatal — Lookup falls back to the per-uuid
// path. Loop runs until ctx is done.
func (s *Service) StartBulkRefresh(ctx context.Context, domain string, hourUTC int) {
	if hourUTC < 0 || hourUTC > 23 {
		hourUTC = DefaultBulkRefreshHourUTC
	}

	ok := s.refreshBulk(ctx, domain)

	go func() {
		for {
			timer := time.NewTimer(bulkRefreshDelay(time.Now(), hourUTC, ok))
			select {
			case <-timer.C:
				ok = s.refreshBulk(ctx, domain)
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
}

// bulkRefreshDelay is how long to wait before the next fetch: until the next
// hourUTC:00, or bulkRefreshRetryInterval if the last one failed — whichever
// comes first, so a retry never pushes the fetch past its daily slot.
func bulkRefreshDelay(now time.Time, hourUTC int, lastOK bool) time.Duration {
	daily := nextDailyUTC(now, hourUTC).Sub(now)
	if lastOK || daily <= bulkRefreshRetryInterval {
		return daily
	}
	return bulkRefreshRetryInterval
}

// nextDailyUTC returns the first instant strictly after now at hourUTC:00 UTC.
// Recomputed each loop iteration so the schedule stays pinned to the wall
// clock and cannot drift by the fetch's own duration.
func nextDailyUTC(now time.Time, hourUTC int) time.Time {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), hourUTC, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// refreshBulk replaces the snapshot, reporting whether the fetch succeeded so
// the caller can retry sooner. A nil scim client counts as success: there is
// nothing to retry.
func (s *Service) refreshBulk(ctx context.Context, domain string) bool {
	if s.scim == nil {
		return true
	}
	users, err := s.scim.ListUsersByDomain(ctx, domain)
	if err != nil {
		// Leave the previous snapshot in place — stale is better than empty,
		// and Lookup already falls back to the per-uuid path for anyone this
		// snapshot doesn't (yet, or ever) know about.
		slog.WarnContext(ctx, "directory: bulk refresh failed, keeping the last known snapshot",
			"domain", domain, "err", err)
		return false
	}

	next := make(map[string]Person, len(users))
	for _, u := range users {
		next[u.UUID] = Person{UUID: u.UUID, Email: u.Email, DisplayName: u.DisplayName}
	}

	s.bulkMu.Lock()
	s.bulk = next
	s.bulkMu.Unlock()
	slog.InfoContext(ctx, "directory: bulk snapshot refreshed", "domain", domain, "count", len(next))
	return true
}

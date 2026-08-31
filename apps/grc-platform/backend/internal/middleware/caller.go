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

package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/routeguard"
)

// externalUserType is the user row's value for an external identity. Anything
// else counts as internal, matching the column's NOT NULL DEFAULT 'INTERNAL'
// and directory.Service.LookupTyped.
const externalUserType = "EXTERNAL"

// emailVerification is the email_verified claim's tri-state. Absent must stay
// distinct from false: reading a missing claim as "not verified" would classify
// every employee as external.
type emailVerification int

const (
	emailVerificationAbsent emailVerification = iota
	emailVerificationTrue
	emailVerificationFalse
)

// String keeps the two non-true cases apart in the rejection log.
func (v emailVerification) String() string {
	switch v {
	case emailVerificationTrue:
		return "true"
	case emailVerificationFalse:
		return "false"
	default:
		return "absent"
	}
}

// parseEmailVerified decodes email_verified, which Asgardeo emits as a bool in
// some configurations and the string "true" in others. An unrecognised shape
// reads as absent rather than locking the caller out.
func parseEmailVerified(raw json.RawMessage) emailVerification {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return emailVerificationAbsent
	}
	switch strings.ToLower(strings.Trim(s, `"`)) {
	case "true":
		return emailVerificationTrue
	case "false":
		return emailVerificationFalse
	}
	return emailVerificationAbsent
}

// emailDomain returns the lowercased domain part of email. Both halves must be
// non-empty: LastIndex returns -1 with no "@", and slicing from idx+1 would
// then yield the whole string, passing a bare "wso2.com" as the domain.
func emailDomain(email string) (string, bool) {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", false
	}
	return strings.ToLower(email[at+1:]), true
}

// internalDomainSet normalises the configured domains for lookup.
func internalDomainSet(domains []string) map[string]struct{} {
	set := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			set[d] = struct{}{}
		}
	}
	return set
}

// callerGuard confines an external caller to the audit surface. Built once per
// Auth; router must be the same mux Auth wraps.
type callerGuard struct {
	router  *http.ServeMux
	domains map[string]struct{}
	enabled bool
}

// isInternal decides from the token alone. Org claims and user_type are not
// consulted. A verified-false address is external even under a corporate
// domain; an absent claim is not held against the caller.
func (g callerGuard) isInternal(email string, verified emailVerification) bool {
	if verified == emailVerificationFalse {
		return false
	}
	domain, ok := emailDomain(email)
	if !ok {
		return false
	}
	_, ok = g.domains[domain]
	return ok
}

// externallyVisible resolves r's matched route and reports whether it is on
// the external allow-list. An unmatched path and a method mismatch both give
// "". A redirect branch gives the real pattern, so a dirty path cleaning onto
// a Risk route is still denied as that route.
func (g callerGuard) externallyVisible(r *http.Request) (pattern string, visible bool) {
	_, pattern = g.router.Handler(r)
	return pattern, routeguard.ExternalVisible(pattern)
}

// evaluate classifies the caller and reports whether to block this request,
// logging the verdict when it blocks. Domain only, never the address — the
// correlation ID already identifies the caller.
func (g callerGuard) evaluate(r *http.Request, email string, verified emailVerification) (internal, blocked bool) {
	internal = g.isInternal(email, verified)
	if !g.enabled || internal {
		return internal, false
	}
	pattern, visible := g.externallyVisible(r)
	if visible {
		return internal, false
	}
	domain, wellFormed := emailDomain(email)
	_, matched := g.domains[domain]
	slog.WarnContext(r.Context(), "auth: external caller blocked",
		"emailDomain", domain,
		"wellFormedEmail", wellFormed,
		"domainMatched", matched,
		// .String() so a JSON handler cannot marshal the int and lose
		// absent-vs-false.
		"emailVerified", verified.String(),
		"pattern", pattern)
	return internal, true
}

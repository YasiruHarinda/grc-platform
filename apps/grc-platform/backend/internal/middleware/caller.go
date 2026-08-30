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
	"strings"
)

// emailVerification is the email_verified claim's tri-state. Absent must stay
// distinct from false: reading a missing claim as "not verified" would classify
// every employee as external.
type emailVerification int

const (
	emailVerificationAbsent emailVerification = iota
	emailVerificationTrue
	emailVerificationFalse
)

// String keeps the two non-true cases distinguishable in the rejection log.
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
// some configurations and the string "true" in others — a plain bool field
// would read the string form as false. An unrecognised shape is treated as
// absent rather than locking the caller out.
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

// emailDomain returns the lowercased domain part of email.
//
// Both halves must be non-empty. LastIndex returns -1 with no "@", and slicing
// from idx+1 would then yield the whole string — a bare "wso2.com" claim would
// pass as the corporate domain.
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

// isInternalCaller decides from the token alone. Org claims and user_type are
// not consulted. A verified-false address is external even under a corporate
// domain; an absent claim is not held against the caller.
func isInternalCaller(email string, verified emailVerification, internal map[string]struct{}) bool {
	if verified == emailVerificationFalse {
		return false
	}
	domain, ok := emailDomain(email)
	if !ok {
		return false
	}
	_, ok = internal[domain]
	return ok
}

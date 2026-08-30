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

// Package routeguard classifies every route by whether an external caller may
// reach it. A second fence behind the per-handler privilege checks.
package routeguard

import "net/http"

// Router is the part of *http.ServeMux route registration uses. An interface so
// the drift test can record patterns; ServeMux will not enumerate its own.
type Router interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// externalVisible answers, per exact route pattern, whether an external caller
// may reach the handler. Every registered pattern must appear here — a test
// fails otherwise, so a new route forces the decision.
//
// Derived from the external auditor's privileges, the only EXTERNAL-assignable
// role today. A second such role means re-deriving this: the test watches
// routes, not roles.
var externalVisible = map[string]bool{
	// ── Bootstrap ────────────────────────────────────────────────────────────
	// Called unconditionally by the app shell; blocking either dead-ends login.
	"GET /api/v1/me/privileges": true,
	"GET /api/v1/me/profile":    true,

	// ── Audit Hub: landing pages ─────────────────────────────────────────────
	"GET /api/v1/audits/dashboard":  true,
	"GET /api/v1/audits/work-queue": true,

	// ── Audit Hub: lookups ───────────────────────────────────────────────────
	// Gated on AUDIT_VIEW_AUDITS, which the auditor holds — not admin-only.
	"GET /api/v1/audits/frameworks":           true,
	"GET /api/v1/audits/frameworks/controls":  true,
	"GET /api/v1/audits/products":             true,
	"GET /api/v1/audits/teams":                true,
	"POST /api/v1/audits/frameworks":          false,
	"POST /api/v1/audits/frameworks/controls": false,
	"POST /api/v1/audits/products":            false,
	"POST /api/v1/audits/teams":               false,
	"PUT /api/v1/audits/teams/{id}":           false,
	"GET /api/v1/audits/users":                false,
	"GET /api/v1/audits/auditor-candidates":   false,

	// ── Audit Hub: ops trigger ───────────────────────────────────────────────
	"POST /api/v1/audits/reminders/run": false,

	// ── Audit Hub: audits ────────────────────────────────────────────────────
	"GET /api/v1/audits":            true,
	"GET /api/v1/audits/{id}":       true,
	"POST /api/v1/audits":           false,
	"PUT /api/v1/audits/{id}":       false,
	"DELETE /api/v1/audits/{id}":    false,
	"GET /api/v1/audits/{id}/trail": true,

	// ── Audit Hub: controls ──────────────────────────────────────────────────
	"GET /api/v1/audits/{id}/controls":                              true,
	"GET /api/v1/audits/{id}/controls/{controlId}":                  true,
	"GET /api/v1/audits/{id}/controls/{controlId}/trail":            true,
	"POST /api/v1/audits/{id}/controls":                             false,
	"POST /api/v1/audits/{id}/controls/bulk":                        false,
	"PUT /api/v1/audits/{id}/controls/{controlId}":                  false,
	"DELETE /api/v1/audits/{id}/controls/{controlId}":               false,
	"PATCH /api/v1/audits/{id}/controls/{controlId}/status":         false,
	"POST /api/v1/audits/{id}/controls/{controlId}/status/override": false,

	// ── Audit Hub: evidence ──────────────────────────────────────────────────
	// Auditor reads and validates; submitting and review belong to the team.
	"GET /api/v1/audits/{id}/controls/{controlId}/evidence":                         true,
	"GET /api/v1/audits/{id}/controls/{controlId}/evidence/files/{fileId}/download": true,
	"POST /api/v1/audits/{id}/controls/{controlId}/evidence/validate":               true,
	"GET /api/v1/audits/{id}/controls/{controlId}/evidence/upload-link":             false,
	"POST /api/v1/audits/{id}/controls/{controlId}/evidence/upload":                 false,
	"POST /api/v1/audits/{id}/controls/{controlId}/evidence/submit":                 false,
	"POST /api/v1/audits/{id}/controls/{controlId}/evidence/files":                  false,
	"POST /api/v1/audits/{id}/controls/{controlId}/evidence/withdraw":               false,
	"POST /api/v1/audits/{id}/controls/{controlId}/evidence/review":                 false,
	"DELETE /api/v1/audits/{id}/controls/{controlId}/evidence/files/{fileId}":       false,
	"DELETE /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}":         false,
	// A pre-existing gap, not a decision: the handler has no assigned-auditor
	// branch. Flip to true when that is fixed.
	"GET /api/v1/audits/{id}/controls/{controlId}/evidence/{evidenceId}/ai-validations": false,

	// ── Audit Hub: population ────────────────────────────────────────────────
	"GET /api/v1/audits/{id}/controls/{controlId}/population":                   true,
	"POST /api/v1/audits/{id}/controls/{controlId}/population/validate":         true,
	"DELETE /api/v1/audits/{id}/controls/{controlId}/population/files/{fileId}": true,
	"GET /api/v1/audits/{id}/controls/{controlId}/population/upload-link":       false,
	"POST /api/v1/audits/{id}/controls/{controlId}/population/upload":           false,
	"POST /api/v1/audits/{id}/controls/{controlId}/population/submit":           false,
	"POST /api/v1/audits/{id}/controls/{controlId}/population/review":           false,
	"DELETE /api/v1/audits/{id}/controls/{controlId}/population/attestation":    false,
	// Same pre-existing gap: no assigned-auditor branch, unlike its evidence
	// sibling, so the auditor cannot download the file they must sample from.
	"GET /api/v1/audits/{id}/controls/{controlId}/population/files/{fileId}/download": false,

	// ── Audit Hub: sampling ──────────────────────────────────────────────────
	"GET /api/v1/audits/{id}/controls/{controlId}/sample/upload-link":   true,
	"POST /api/v1/audits/{id}/controls/{controlId}/sample/upload":       true,
	"POST /api/v1/audits/{id}/controls/{controlId}/sample/submit":       true,
	"POST /api/v1/audits/{id}/controls/{controlId}/sample/request-time": true,

	// ── Audit Hub: comments ──────────────────────────────────────────────────
	// Internal threads and others' comments are filtered inside the handler.
	"GET /api/v1/audits/{id}/controls/{controlId}/comments":                true,
	"POST /api/v1/audits/{id}/controls/{controlId}/comments":               true,
	"DELETE /api/v1/audits/{id}/controls/{controlId}/comments/{commentId}": true,

	// ── Risk Hub ─────────────────────────────────────────────────────────────
	"GET /api/v1/risks/teams":                                       false,
	"POST /api/v1/risks/teams":                                      false,
	"PUT /api/v1/risks/teams/{id}":                                  false,
	"GET /api/v1/risks/scores":                                      false,
	"GET /api/v1/risks/compliance-references":                       false,
	"POST /api/v1/risks/compliance-references":                      false,
	"PUT /api/v1/risks/compliance-references/{id}":                  false,
	"DELETE /api/v1/risks/compliance-references/{id}":               false,
	"GET /api/v1/risks/categories":                                  false,
	"POST /api/v1/risks/categories":                                 false,
	"PUT /api/v1/risks/categories/{id}":                             false,
	"DELETE /api/v1/risks/categories/{id}":                          false,
	"GET /api/v1/risks/users":                                       false,
	"POST /api/v1/risks/users/resolve":                              false,
	"GET /api/v1/risks/management-approvers":                        false,
	"GET /api/v1/risks/owner-candidates":                            false,
	"GET /api/v1/risks/assigner-candidates":                         false,
	"GET /api/v1/risks/employees/search":                            false,
	"GET /api/v1/risks/next-sequence-id":                            false,
	"GET /api/v1/risks":                                             false,
	"POST /api/v1/risks":                                            false,
	"GET /api/v1/risks/{id}":                                        false,
	"PUT /api/v1/risks/{id}":                                        false,
	"POST /api/v1/risks/{id}/owner-approve":                         false,
	"POST /api/v1/risks/{id}/management-approve":                    false,
	"POST /api/v1/risks/{id}/approve":                               false,
	"POST /api/v1/risks/{id}/reject":                                false,
	"POST /api/v1/risks/{id}/complete":                              false,
	"POST /api/v1/risks/{id}/resubmit":                              false,
	"POST /api/v1/risks/{id}/close":                                 false,
	"POST /api/v1/risks/{id}/cancel":                                false,
	"POST /api/v1/risks/{id}/assess":                                false,
	"GET /api/v1/risks/dashboard":                                   false,
	"GET /api/v1/risks/analytics/summary":                           false,
	"POST /api/v1/risks/{id}/action-plans":                          false,
	"GET /api/v1/risks/{id}/action-plans":                           false,
	"GET /api/v1/risks/{id}/action-plans/{planId}/steps":            false,
	"PATCH /api/v1/risks/{id}/action-plans/{planId}/steps/{stepId}": false,
	"POST /api/v1/risks/{id}/action-plans/{planId}/complete":        false,
	"POST /api/v1/risks/{id}/escalate":                              false,
	"GET /api/v1/risks/{id}/escalations":                            false,
	"POST /api/v1/risks/escalations/run":                            false,
	"GET /api/v1/risks/{id}/history":                                false,
	"POST /api/v1/risks/{id}/escalations/{escalationId}/comment":    false,
	"POST /api/v1/risks/{id}/evidence":                              false,
	"GET /api/v1/risks/{id}/evidence":                               false,
	"DELETE /api/v1/risks/{id}/evidence/{fileId}":                   false,
	"GET /api/v1/risks/{id}/evidence/{fileId}/download":             false,

	// ── Admin Console ────────────────────────────────────────────────────────
	"GET /api/v1/admin/directory/search":               false,
	"GET /api/v1/admin/directory/search-external":      false,
	"POST /api/v1/admin/users":                         false,
	"GET /api/v1/admin/users":                          false,
	"PATCH /api/v1/admin/users/{id}/status":            false,
	"POST /api/v1/admin/users/{id}/grants":             false,
	"DELETE /api/v1/admin/users/{id}/grants/{grantId}": false,
	"GET /api/v1/admin/roles":                          false,
}

// ExternalVisible reports whether an external caller may reach pattern. An
// unknown pattern — including the "" the mux returns for an unmatched path or
// method mismatch — is not visible.
func ExternalVisible(pattern string) bool {
	return externalVisible[pattern]
}

// Patterns returns a copy of the map, for the drift test.
func Patterns() map[string]bool {
	out := make(map[string]bool, len(externalVisible))
	for k, v := range externalVisible {
		out[k] = v
	}
	return out
}

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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import "testing"

// TestBackendWorkflowTransitionsAreAllowed asserts that every status change the
// GRC backend's risk service actually performs is permitted here.
//
// The backend owns the risk workflow; this map only guards against nonsense. If
// a case below fails, the map is wrong, not the backend — a rejected transition
// surfaces to the user as a 409 on a legitimate action. An earlier version of
// the map failed most of these.
func TestBackendWorkflowTransitionsAreAllowed(t *testing.T) {
	cases := []struct {
		op       string
		from, to string
	}{
		// OwnerApprove — the ordinary path, and the ACCEPT+HIGH detour.
		{"OwnerApprove", "PENDING_RISK_OWNER_APPROVAL", "PENDING_COMPLIANCE_REVIEW"},
		{"OwnerApprove (ACCEPT+HIGH)", "PENDING_RISK_OWNER_APPROVAL", "PENDING_MANAGEMENT_APPROVAL"},
		{"OwnerApprove after amendment", "PENDING_AMENDMENT", "PENDING_COMPLIANCE_REVIEW"},
		{"OwnerApprove after amendment (ACCEPT+HIGH)", "PENDING_AMENDMENT", "PENDING_MANAGEMENT_APPROVAL"},
		{"OwnerApprove completion", "PENDING_OWNER_COMPLETION_APPROVAL", "PENDING_COMPLIANCE_CLOSURE"},

		{"ManagementApprove", "PENDING_MANAGEMENT_APPROVAL", "PENDING_COMPLIANCE_REVIEW"},
		{"Approve", "PENDING_COMPLIANCE_REVIEW", "IN_REMEDIATION"},
		{"Complete", "IN_REMEDIATION", "PENDING_OWNER_COMPLETION_APPROVAL"},
		{"Close", "PENDING_COMPLIANCE_CLOSURE", "CLOSED"},
		{"Cancel", "PENDING_RISK_OWNER_APPROVAL", "CANCELLED"},

		// Reject — from every stage the backend allows rejection at.
		{"Reject (OWNER)", "PENDING_RISK_OWNER_APPROVAL", "PENDING_REVISION"},
		{"Reject (OWNER, amendment)", "PENDING_AMENDMENT", "PENDING_REVISION"},
		{"Reject (MANAGEMENT)", "PENDING_MANAGEMENT_APPROVAL", "PENDING_REVISION"},
		{"Reject (COMPLIANCE)", "PENDING_COMPLIANCE_REVIEW", "PENDING_REVISION"},
		{"Reject (COMPLETION_OWNER)", "PENDING_OWNER_COMPLETION_APPROVAL", "PENDING_REVISION"},

		// Resubmit — ordinary, and the COMPLETION_OWNER variant.
		{"Resubmit", "PENDING_REVISION", "PENDING_RISK_OWNER_APPROVAL"},
		{"Resubmit (COMPLETION_OWNER)", "PENDING_REVISION", "PENDING_OWNER_COMPLETION_APPROVAL"},
	}

	for _, c := range cases {
		if !isValidRiskTransition(c.from, c.to) {
			t.Errorf("%s: %s → %s must be allowed but was rejected", c.op, c.from, c.to)
		}
	}
}

// TestTerminalStatusesAreTerminal pins the one thing the map must forbid.
func TestTerminalStatusesAreTerminal(t *testing.T) {
	for _, from := range []string{"CLOSED", "CANCELLED"} {
		for _, to := range []string{"PENDING_RISK_OWNER_APPROVAL", "IN_REMEDIATION", "PENDING_REVISION"} {
			if isValidRiskTransition(from, to) {
				t.Errorf("%s is terminal but %s → %s was allowed", from, from, to)
			}
		}
	}
}

// TestNoOpAndUnknownFrom documents two deliberate escape hatches: a no-op
// transition always passes, and an empty current status is not policed.
func TestNoOpAndUnknownFrom(t *testing.T) {
	if !isValidRiskTransition("IN_REMEDIATION", "IN_REMEDIATION") {
		t.Error("a no-op transition must be allowed")
	}
	if !isValidRiskTransition("", "PENDING_COMPLIANCE_REVIEW") {
		t.Error("an empty current status must not be policed")
	}
}

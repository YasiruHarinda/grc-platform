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

package model

import "slices"

// Workflow status constants for the risk lifecycle state machine.
// Defining them here gives the compiler visibility over every transition and
// prevents silent drift from bare string literals scattered across packages.
const (
	StatusPendingOwnerApproval      = "PENDING_RISK_OWNER_APPROVAL"
	StatusPendingManagementApproval = "PENDING_MANAGEMENT_APPROVAL"
	StatusPendingComplianceReview   = "PENDING_COMPLIANCE_REVIEW"
	StatusInRemediation             = "IN_REMEDIATION"
	StatusPendingOwnerCompletion    = "PENDING_OWNER_COMPLETION_APPROVAL"
	// StatusPendingManagementClosure is the closure-path counterpart of
	// StatusPendingManagementApproval: an ACCEPT + HIGH risk needs the same
	// named Management Approver to sign off on the finished remediation before
	// Compliance closes it. Non-ACCEPT/HIGH risks skip it entirely.
	StatusPendingManagementClosure = "PENDING_MANAGEMENT_CLOSURE_APPROVAL"
	StatusPendingComplianceClosure = "PENDING_COMPLIANCE_CLOSURE"
	StatusPendingAmendment         = "PENDING_AMENDMENT"
	StatusPendingRevision          = "PENDING_REVISION"
	StatusEscalated                = "ESCALATED"
	StatusClosed                   = "CLOSED"
	StatusCancelled                = "CANCELLED"
)

// AllWorkflowStatuses is every value the workflow_status column can hold, in
// lifecycle order. It must name every constant above; status_test.go fails if
// the two drift.
var AllWorkflowStatuses = []string{
	StatusPendingOwnerApproval,
	StatusPendingManagementApproval,
	StatusPendingComplianceReview,
	StatusInRemediation,
	StatusPendingOwnerCompletion,
	StatusPendingManagementClosure,
	StatusPendingComplianceClosure,
	StatusPendingAmendment,
	StatusPendingRevision,
	StatusEscalated,
	StatusClosed,
	StatusCancelled,
}

// TerminalWorkflowStatuses are the statuses that end a risk's life: no further
// work is possible and nobody needs reassigning onto it.
var TerminalWorkflowStatuses = []string{StatusClosed, StatusCancelled}

// IsOngoingWorkflowStatus reports whether a risk in this status still counts as
// live work — the risk counterpart of audit's Audit.IsOngoing(). Defined by
// exclusion, so a new non-terminal status is ongoing with no change here.
func IsOngoingWorkflowStatus(status string) bool {
	return !slices.Contains(TerminalWorkflowStatuses, status)
}

// OngoingWorkflowStatuses is every non-terminal status, for callers that must
// pass an inclusive status filter (the risk list query has no "exclude" form).
// Derived from AllWorkflowStatuses so it cannot silently omit one the way a
// hand-maintained list could.
func OngoingWorkflowStatuses() []string {
	out := make([]string, 0, len(AllWorkflowStatuses))
	for _, s := range AllWorkflowStatuses {
		if IsOngoingWorkflowStatus(s) {
			out = append(out, s)
		}
	}
	return out
}

// RiskTypeNew and RiskTypeUpdated are the two values for the risk_type column.
const (
	RiskTypeNew     = "NEW"
	RiskTypeUpdated = "UPDATED"
)

// The three values for the identified_by_type column.
const (
	IdentifiedByEmployee       = "EMPLOYEE"
	IdentifiedByExternalPerson = "EXTERNAL_PERSON"
	IdentifiedByTool           = "TOOL"
)

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

// Workflow status constants for the risk lifecycle state machine.
// Defining them here gives the compiler visibility over every transition and
// prevents silent drift from bare string literals scattered across packages.
const (
	StatusPendingOwnerApproval      = "PENDING_RISK_OWNER_APPROVAL"
	StatusPendingManagementApproval = "PENDING_MANAGEMENT_APPROVAL"
	StatusPendingComplianceReview   = "PENDING_COMPLIANCE_REVIEW"
	StatusInRemediation             = "IN_REMEDIATION"
	StatusPendingOwnerCompletion    = "PENDING_OWNER_COMPLETION_APPROVAL"
	StatusPendingComplianceClosure  = "PENDING_COMPLIANCE_CLOSURE"
	StatusPendingAmendment          = "PENDING_AMENDMENT"
	StatusPendingRevision           = "PENDING_REVISION"
	StatusEscalated                 = "ESCALATED"
	StatusClosed                    = "CLOSED"
	StatusCancelled                 = "CANCELLED"
)

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

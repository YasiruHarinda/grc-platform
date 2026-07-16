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

// Analytics-page-specific constants. Shared palette/label helpers (treatment,
// level, cert colors) live in ../dashboard/constants and are reused here so
// both pages read as one visual system.

// Fixed pipeline order for the workflow status funnel; exception statuses
// (revision/amendment/escalation) trail the main flow. Statuses absent from
// the response are skipped, not zero-filled — an empty stage isn't a stage.
export const WORKFLOW_FUNNEL_ORDER = [
  "PENDING_RISK_OWNER_APPROVAL",
  "PENDING_MANAGEMENT_APPROVAL",
  "PENDING_COMPLIANCE_REVIEW",
  "IN_REMEDIATION",
  "PENDING_OWNER_COMPLETION_APPROVAL",
  "PENDING_COMPLIANCE_CLOSURE",
  "CLOSED",
  "PENDING_AMENDMENT",
  "PENDING_REVISION",
  "ESCALATED",
] as const;

export const WORKFLOW_STATUS_LABELS: Record<string, string> = {
  PENDING_RISK_OWNER_APPROVAL: "Pending Owner Approval",
  PENDING_MANAGEMENT_APPROVAL: "Pending Management Approval",
  PENDING_COMPLIANCE_REVIEW: "Pending Compliance Approval",
  IN_REMEDIATION: "In Remediation",
  PENDING_OWNER_COMPLETION_APPROVAL: "Awaiting Owner Sign-off",
  PENDING_COMPLIANCE_CLOSURE: "Awaiting Closure",
  PENDING_AMENDMENT: "Pending Amendment",
  PENDING_REVISION: "Pending Revision",
  ESCALATED: "Escalated",
  CLOSED: "Closed",
};

export const WORKFLOW_STAGE_COLOR = "#2a78d6";

// Trend chart series colors. Bars share the treatment-strategy blue/green
// pairing used elsewhere; the score line gets its own accent so it reads
// distinctly against the bars.
export const IDENTIFIED_COLOR = "#2a78d6";
export const CLOSED_TREND_COLOR = "#1baf7a";
export const AVG_SCORE_COLOR = "#eb6834";

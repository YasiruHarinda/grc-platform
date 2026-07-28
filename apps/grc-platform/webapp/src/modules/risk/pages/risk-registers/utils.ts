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

import type { ChipProps } from "@wso2/oxygen-ui";

export type StatusColor = ChipProps["color"];

interface StatusCfg {
  label: string;
  color: StatusColor;
  sx?: Record<string, unknown>;
}

export const STATUS_CONFIG: Record<string, StatusCfg> = {
  PENDING_RISK_OWNER_APPROVAL:    { label: "Pending Owner Approval",      color: "warning" },
  PENDING_MANAGEMENT_APPROVAL:    { label: "Pending Management Approval",  color: "warning" },
  PENDING_COMPLIANCE_REVIEW:      { label: "Pending Compliance Approval",  color: "default" },
  IN_REMEDIATION:                 { label: "In Remediation",               color: "info" },
  PENDING_OWNER_COMPLETION_APPROVAL: { label: "Awaiting Owner Sign-off",   color: "warning" },
  PENDING_COMPLIANCE_CLOSURE:     { label: "Awaiting Closure",             color: "default", sx: { bgcolor: "#c8e6c9", color: "#1b5e20" } },
  PENDING_AMENDMENT:              { label: "Pending Amendment",            color: "warning" },
  PENDING_REVISION:               { label: "Pending Revision",             color: "error" },
  ESCALATED:                      { label: "Escalated",                    color: "error" },
  CLOSED:                         { label: "Closed",                       color: "default", sx: { bgcolor: "#388e3c", color: "#fff" } },
};

// Parses a date-only value as local time, whether it's a bare "YYYY-MM-DD"
// or the backend's serialized UTC-midnight timestamp form
// ("YYYY-MM-DDT00:00:00Z"). Reading the calendar digits directly and
// constructing a local Date from them sidesteps new Date(s)'s UTC
// interpretation, which otherwise drifts the displayed day back by one in
// any timezone west of UTC.
export function parseDateStr(s: string): Date {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(s);
  if (m) {
    return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  }
  return new Date(s);
}

// Calculates the age in days from the risk_identified_date or created_at string.
export function calcAge(dateStr: string | null | undefined): number {
  if (!dateStr) return 0;
  const then = parseDateStr(dateStr);
  const now = new Date();
  return Math.max(0, Math.floor((now.getTime() - then.getTime()) / 86_400_000));
}

export interface DueInfo {
  label: string;
  color: string;
  daysLeft: number;
}

// Returns due/overdue label and color based on implementation_date.
// color thresholds: overdue = red, <=7 days = orange, >7 days = green.
// Meaningless for a CLOSED risk (nothing is "due" anymore) — callers should
// check workflow_status themselves and skip calling this entirely rather
// than passing a reference date, so this stays a pure "today" calculation.
export function calcDue(implementationDate: string | null | undefined): DueInfo {
  if (!implementationDate) return { label: "—", color: "text.secondary", daysLeft: 0 };
  const due = parseDateStr(implementationDate);
  const now = new Date();
  due.setHours(0, 0, 0, 0);
  now.setHours(0, 0, 0, 0);
  const diff = Math.round((due.getTime() - now.getTime()) / 86_400_000);
  if (diff < 0) {
    return { label: `Overdue ${Math.abs(diff)}d`, color: "error.main", daysLeft: diff };
  }
  if (diff === 0) {
    return { label: "Due today", color: "warning.main", daysLeft: 0 };
  }
  if (diff <= 7) {
    return { label: `Due in ${diff}d`, color: "warning.main", daysLeft: diff };
  }
  return { label: `Due in ${diff}d`, color: "success.main", daysLeft: diff };
}

// Formats an ISO date/datetime string (e.g. "2026-06-30" or "2026-06-30T00:00:00Z")
// into a readable form: "30 Jun 2026".
export function formatDate(dateStr: string | null | undefined): string {
  if (!dateStr) return "—";
  const d = parseDateStr(dateStr);
  if (isNaN(d.getTime())) return dateStr;
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
}

// Statuses for each pending-approval tab.
export const PENDING_OWNER_STATUSES = [
  "PENDING_RISK_OWNER_APPROVAL",
  "PENDING_AMENDMENT",
  "PENDING_OWNER_COMPLETION_APPROVAL",
];

export const PENDING_MANAGEMENT_STATUSES = [
  "PENDING_MANAGEMENT_APPROVAL",
];

export const PENDING_COMPLIANCE_STATUSES = [
  "PENDING_COMPLIANCE_REVIEW",
  "PENDING_COMPLIANCE_CLOSURE",
];

export const PENDING_REVISION_STATUSES = [
  "PENDING_REVISION",
];

// Statuses that appear in the "Approved Risks" tab.
export const APPROVED_OPEN_STATUSES = ["IN_REMEDIATION"];

// ESCALATED risks stay visible here too (their chip still reads "Open" — see
// statusLabel()/OutlinedStatusChip() in RiskRegisters.tsx, which special-case
// ESCALATED for this tab specifically). The dedicated "Overdue Risks" tab is
// the only place that shows the real "Escalated" chip.
export const APPROVED_ALL_STATUSES = [...APPROVED_OPEN_STATUSES, "CLOSED", "ESCALATED"];

// Statuses shown in the "Overdue Risks" tab.
export const OVERDUE_STATUSES = ["ESCALATED"];

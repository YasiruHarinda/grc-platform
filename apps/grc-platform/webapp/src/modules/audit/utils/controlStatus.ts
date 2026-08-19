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

import type { ControlStatus } from "@modules/audit/types/audit";

export const CONTROL_STATUS_LABELS: Record<ControlStatus, string> = {
  POPULATION_PENDING:            "Population Pending",
  POPULATION_INTERNAL_REVIEW:    "Population Internal Review",
  POPULATION_UNDER_VALIDATION:   "Population Under Validation",
  POPULATION_NEED_CLARIFICATION: "Population Need Clarification",
  POPULATION_COMPLETE:           "Population Complete",
  AWAITING_SAMPLE:               "Awaiting Sample",
  SUBMITTED_SAMPLE:              "Submitted Sample",
  EVIDENCE_PENDING:              "Evidence Pending",
  EVIDENCE_INTERNAL_REVIEW:      "Evidence Internal Review",
  EVIDENCE_UNDER_VALIDATION:     "Evidence Under Validation",
  EVIDENCE_NEED_CLARIFICATION:   "Evidence Need Clarification",
  COMPLETE:                      "Complete",
};

// ── 4-phase rollup for the dashboard donut ───────────────────────────────────
// Groups the 12 statuses into scannable phases; the donut offers a "Detailed"
// toggle that switches back to the full per-status breakdown.

export type ControlPhase = "NOT_STARTED" | "IN_PROGRESS" | "BLOCKED" | "COMPLETE";

export const STATUS_PHASE: Record<ControlStatus, ControlPhase> = {
  POPULATION_PENDING:            "NOT_STARTED",
  EVIDENCE_PENDING:              "NOT_STARTED",
  POPULATION_INTERNAL_REVIEW:    "IN_PROGRESS",
  POPULATION_UNDER_VALIDATION:   "IN_PROGRESS",
  POPULATION_COMPLETE:           "IN_PROGRESS",
  AWAITING_SAMPLE:               "IN_PROGRESS",
  SUBMITTED_SAMPLE:              "IN_PROGRESS",
  EVIDENCE_INTERNAL_REVIEW:      "IN_PROGRESS",
  EVIDENCE_UNDER_VALIDATION:     "IN_PROGRESS",
  POPULATION_NEED_CLARIFICATION: "BLOCKED",
  EVIDENCE_NEED_CLARIFICATION:   "BLOCKED",
  COMPLETE:                      "COMPLETE",
};

export const PHASE_LABELS: Record<ControlPhase, string> = {
  NOT_STARTED: "Not Started",
  IN_PROGRESS: "In Progress",
  BLOCKED:     "Needs Clarification",
  COMPLETE:    "Complete",
};

export const PHASE_COLORS: Record<ControlPhase, string> = {
  NOT_STARTED: "#94A3B8", // slate  — neutral / not started
  IN_PROGRESS: "#6366F1", // indigo — active work
  BLOCKED:     "#EF4444", // red    — needs clarification
  COMPLETE:    "#10B981", // emerald — done
};

export const PHASE_ORDER: ControlPhase[] = ["NOT_STARTED", "IN_PROGRESS", "BLOCKED", "COMPLETE"];

// ── Backward status override ─────────────────────────────────────────────
// Mirrors the entity's controlStatusRank (audit_control_service.go): a single
// ordered list rather than a second transition map. An override to `target`
// is only offered when rank(target) < rank(current) — DESIGN controls are
// floored at CONTROL_STATUS_RANK.EVIDENCE_PENDING since they have no
// population row to rewind into.

export const CONTROL_STATUS_RANK: Record<ControlStatus, number> = {
  POPULATION_PENDING:            0,
  POPULATION_INTERNAL_REVIEW:    1,
  POPULATION_NEED_CLARIFICATION: 1,
  POPULATION_UNDER_VALIDATION:   2,
  POPULATION_COMPLETE:           3,
  AWAITING_SAMPLE:               4,
  SUBMITTED_SAMPLE:              5,
  EVIDENCE_PENDING:              6,
  EVIDENCE_NEED_CLARIFICATION:   6,
  EVIDENCE_INTERNAL_REVIEW:      7,
  EVIDENCE_UNDER_VALIDATION:     8,
  COMPLETE:                      9,
};

const DESIGN_CONTROL_FLOOR_RANK = CONTROL_STATUS_RANK.EVIDENCE_PENDING;

// overridableStatuses returns every status the given control could be
// backward-overridden to: strictly lower rank than `current`, floored for
// DESIGN controls (they have no audit_population row to rewind into).
export function overridableStatuses(current: ControlStatus, requirementType: "DESIGN" | "OE"): ControlStatus[] {
  const currentRank = CONTROL_STATUS_RANK[current];
  const floor = requirementType === "DESIGN" ? DESIGN_CONTROL_FLOOR_RANK : 0;
  return (Object.keys(CONTROL_STATUS_RANK) as ControlStatus[]).filter(
    (status) => CONTROL_STATUS_RANK[status] < currentRank && CONTROL_STATUS_RANK[status] >= floor,
  );
}

export const CONTROL_STATUS_COLORS: Record<ControlStatus, string> = {
  // OE population phase
  POPULATION_PENDING:            "#94A3B8", // slate   — not started
  POPULATION_INTERNAL_REVIEW:    "#F59E0B", // amber   — under internal review
  POPULATION_UNDER_VALIDATION:   "#8B5CF6", // violet  — auditor reviewing
  POPULATION_NEED_CLARIFICATION: "#EF4444", // red     — blocked
  POPULATION_COMPLETE:           "#14B8A6", // teal    — population approved
  AWAITING_SAMPLE:               "#14B8A6", // teal    — waiting on auditor (same phase)
  SUBMITTED_SAMPLE:              "#6366F1", // indigo  — sample sent, team to submit evidence
  // Evidence phase
  EVIDENCE_PENDING:              "#F59E0B", // amber   — team yet to submit
  EVIDENCE_INTERNAL_REVIEW:      "#6366F1", // indigo  — under internal review
  EVIDENCE_UNDER_VALIDATION:     "#8B5CF6", // violet  — auditor reviewing
  EVIDENCE_NEED_CLARIFICATION:   "#EF4444", // red     — blocked
  COMPLETE:                      "#10B981", // emerald — approved & closed
};

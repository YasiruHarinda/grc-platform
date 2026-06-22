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

import type { FilterField } from "@modules/audit/components/FilterPanel";
import { CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";
import type { AuditControl } from "@modules/audit/types/audit";

export const EMPTY_CONTROL_FILTERS: Record<string, string[]> = {
  isOverdue: [],
  status: [],
  teamName: [],
  requirementType: [],
  controlType: [],
  scope: [],
};

/** Returns the number of filter fields that have at least one value selected. */
export function activeFilterCount(filters: Record<string, string[]>): number {
  return Object.values(filters).filter((arr) => arr.length > 0).length;
}

/**
 * Filters controls by all active fields (AND across fields, OR within each field)
 * and an optional free-text search against control number and description.
 */
export function applyControlFilters(
  controls: AuditControl[],
  filters: Record<string, string[]>,
  search = "",
): AuditControl[] {
  const q = search.trim().toLowerCase();
  return controls.filter((c) => {
    if (q) {
      const matchesNumber = c.controlNumber.toLowerCase().includes(q);
      const matchesDesc = c.description.toLowerCase().includes(q);
      if (!matchesNumber && !matchesDesc) return false;
    }
    if (filters.isOverdue?.includes("true") && !c.isOverdue) return false;
    if (filters.status?.length && !filters.status.includes(c.status)) return false;
    if (filters.requirementType?.length && !filters.requirementType.includes(c.requirementType)) return false;
    if (filters.controlType?.length && !filters.controlType.includes(c.controlType)) return false;
    if (filters.scope?.length && !filters.scope.includes(c.scope)) return false;
    if (filters.teamName?.length && !filters.teamName.includes(c.teamName ?? "")) return false;
    return true;
  });
}

/** Derives the FilterField definitions for the controls filter panel.
 *  The team field is built dynamically from the actual controls in the current audit. */
export function buildControlFilterFields(controls: AuditControl[]): FilterField[] {
  const uniqueTeams = [
    ...new Set(controls.map((c) => c.teamName).filter((v): v is string => v !== null)),
  ].sort();

  return [
    {
      id: "isOverdue",
      label: "Overdue",
      options: [{ label: "Overdue only", value: "true" }],
    },
    {
      id: "status",
      label: "Status",
      options: (
        [
          "SAMPLING_REQUIRED",
          "NOT_STARTED",
          "EVIDENCE_SUBMITTED",
          "COMPLIANCE_REVIEW",
          "AUDITOR_REVIEW",
          "APPROVED",
          "RESUBMIT_REQUIRED",
        ] as const
      ).map((s) => ({ label: CONTROL_STATUS_LABELS[s], value: s })),
    },
    {
      id: "teamName",
      label: "Team",
      options: uniqueTeams.map((v) => ({ label: v, value: v })),
    },
    {
      id: "requirementType",
      label: "Req. Type",
      options: [
        { label: "Design", value: "DESIGN" },
        { label: "Operative Effectiveness (OE)", value: "OE" },
      ],
    },
    {
      id: "controlType",
      label: "Control Type",
      options: [
        { label: "Config", value: "CONFIG" },
        { label: "Non-Config", value: "NON_CONFIG" },
      ],
    },
    {
      id: "scope",
      label: "Scope",
      options: [
        { label: "Common", value: "COMMON" },
        { label: "Product Specific", value: "PRODUCT_SPECIFIC" },
      ],
    },
  ];
}

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

import { Chip } from "@wso2/oxygen-ui";
import type { JSX } from "react";

// Round status (distinct from the control's status) — tells a rejected round
// apart from the resubmission that replaced it. Shared by evidence and
// population, which use the same round statuses.
const ROUND_STATUS_LABELS: Record<string, string> = {
  SUBMITTED:           "Submitted",
  COMPLIANCE_APPROVED: "Approved (Internal)",
  COMPLIANCE_REJECTED: "Rejected (Internal)",
  APPROVED:            "Approved",
  AUDITOR_REJECTED:    "Rejected (Auditor)",
};
const ROUND_STATUS_COLORS: Record<string, string> = {
  SUBMITTED:           "#6366F1", // indigo — awaiting review
  COMPLIANCE_APPROVED:  "#10B981", // emerald
  COMPLIANCE_REJECTED:  "#EF4444", // red
  APPROVED:             "#10B981", // emerald
  AUDITOR_REJECTED:     "#EF4444", // red
};

/** Chip for a round's own status; renders nothing for a status with no label (e.g. PENDING). */
export default function RoundStatusChip({ status }: { status: string }): JSX.Element | null {
  if (!ROUND_STATUS_LABELS[status]) return null;
  return (
    <Chip
      label={ROUND_STATUS_LABELS[status]}
      size="small"
      variant="outlined"
      sx={{
        height: 18,
        fontSize: "0.65rem",
        fontWeight: 600,
        color: ROUND_STATUS_COLORS[status],
        borderColor: ROUND_STATUS_COLORS[status],
        "& .MuiChip-label": { px: 0.75 },
      }}
    />
  );
}

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
import type { RoundStatus } from "@modules/audit/types/audit";
import { ROUND_STATUS_COLORS, ROUND_STATUS_LABELS } from "@modules/audit/utils/controlStatus";

/**
 * Chip for a round's own status — tells a rejected round apart from the
 * resubmission that replaced it. Shared by evidence and population, which use
 * the same round statuses. Renders nothing for PENDING (nothing submitted yet)
 * or for a status this build doesn't know, rather than an empty chip.
 */
export default function RoundStatusChip({ status }: { status: string }): JSX.Element | null {
  if (status === "PENDING") return null;
  const label = ROUND_STATUS_LABELS[status as RoundStatus];
  if (!label) return null;
  return (
    <Chip
      label={label}
      size="small"
      variant="outlined"
      sx={{
        height: 18,
        fontSize: "0.65rem",
        fontWeight: 600,
        color: ROUND_STATUS_COLORS[status as RoundStatus],
        borderColor: ROUND_STATUS_COLORS[status as RoundStatus],
        "& .MuiChip-label": { px: 0.75 },
      }}
    />
  );
}

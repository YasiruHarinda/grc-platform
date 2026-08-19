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

import { Box, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { ScopeRollup } from "@modules/audit/types/framework";
import { DUE_SOON } from "@modules/audit/components/dashboard/dueDate";
import CompletionRing from "@modules/audit/components/dashboard/CompletionRing";

const PHASE_ROWS: { key: keyof ScopeRollup["phaseCounts"]; label: string }[] = [
  { key: "complete", label: "Complete" },
  { key: "inProgress", label: "In Progress" },
  { key: "needsClarification", label: "Needs Clarification" },
  { key: "notStarted", label: "Not Started" },
];

interface StandPanelProps {
  scope: ScopeRollup;
}

// "Where we stand" — completion ring, four-phase breakdown.
// No audit-period-elapsed bar (removed at the client's request); pace
// still drives the status rule, it just isn't drawn here.
export default function StandPanel({ scope }: StandPanelProps): JSX.Element {
  const total = scope.total;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2, height: "100%" }}>
      <Box sx={{ display: "flex", justifyContent: "center" }}>
        <CompletionRing percent={scope.completionPercent} size={112} />
      </Box>

      <Box sx={{ display: "flex", flexDirection: "column", gap: 1, flex: 1 }}>
        {PHASE_ROWS.map(({ key, label }) => {
          const count = scope.phaseCounts[key];
          const share = total > 0 ? Math.round((count / total) * 100) : 0;
          const colored = key === "needsClarification" && count > 0;
          return (
            <Box key={key} sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <Typography variant="body2" sx={{ color: colored ? DUE_SOON : "text.primary", fontWeight: colored ? 700 : 400 }}>
                {label}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                {count} · {share}%
              </Typography>
            </Box>
          );
        })}
      </Box>
    </Box>
  );
}

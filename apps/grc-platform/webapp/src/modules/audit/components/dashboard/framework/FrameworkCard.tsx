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

import { Box, LinearProgress, Paper, Typography, useTheme } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { FrameworkRollup } from "@modules/audit/types/framework";
import { DUE_OVERDUE } from "@modules/audit/components/dashboard/dueDate";

interface FrameworkCardProps {
  rollup: FrameworkRollup;
  selected: boolean;
  onSelect: () => void;
}

export default function FrameworkCard({ rollup, selected, onSelect }: FrameworkCardProps): JSX.Element {
  const theme = useTheme();
  const completeColor = rollup.completionPercent >= 100 ? theme.palette.success.main : theme.palette.primary.main;

  return (
    <Paper
      variant="outlined"
      role="tab"
      aria-selected={selected}
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onSelect(); } }}
      sx={{
        borderRadius: 2,
        p: 2,
        cursor: "pointer",
        display: "flex",
        flexDirection: "column",
        gap: 1,
        borderColor: selected ? "warning.main" : "divider",
        borderWidth: selected ? 2 : 1,
        transition: "border-color 0.15s, border-width 0.15s",
        "&:hover": { borderColor: selected ? "warning.main" : "text.secondary" },
      }}
    >
      <Typography variant="subtitle1" fontWeight={700} noWrap title={rollup.name} sx={{ minWidth: 0 }}>
        {rollup.name}
      </Typography>

      <Box sx={{ display: "flex", alignItems: "baseline", gap: 0.75, flexWrap: "wrap" }}>
        <Typography variant="h5" fontWeight={700} lineHeight={1}>
          {Math.round(rollup.completionPercent)}%
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {rollup.complete}/{rollup.total} controls · {rollup.auditCount} audit{rollup.auditCount === 1 ? "" : "s"}
        </Typography>
      </Box>

      <LinearProgress
        variant="determinate"
        value={Math.min(rollup.completionPercent, 100)}
        sx={{
          height: 8, borderRadius: 4, bgcolor: "#E0E0E0",
          "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
          "& .MuiLinearProgress-bar": { bgcolor: completeColor, borderRadius: 4 },
        }}
      />

      <Box>
        {rollup.overdueCount > 0 ? (
          <Typography variant="body2" fontWeight={700} sx={{ color: DUE_OVERDUE }}>
            {rollup.overdueCount} overdue
          </Typography>
        ) : (
          <Typography variant="body2" color="text.secondary">No overdue controls</Typography>
        )}
      </Box>
    </Paper>
  );
}

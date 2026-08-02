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

import { Box, LinearProgress, Typography, useTheme } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { FrameworkRollup, ScopeRollup } from "@modules/audit/types/framework";
import { DUE_OVERDUE } from "@modules/audit/components/dashboard/dueDate";
import { formatDaysLeft } from "@modules/audit/utils/frameworkRollup";

interface ChipProps {
  label: string;
  endDateLabel: string;
  rollup: ScopeRollup;
  selected: boolean;
  onSelect: () => void;
}

function AuditChip({ label, endDateLabel, rollup, selected, onSelect }: ChipProps): JSX.Element {
  const theme = useTheme();
  const color = rollup.completionPercent >= 100 ? theme.palette.success.main : theme.palette.primary.main;

  return (
    <Box
      role="tab"
      aria-selected={selected}
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onSelect(); } }}
      sx={{
        cursor: "pointer",
        flexShrink: 0,
        minWidth: 180,
        borderRadius: 1.5,
        border: "1px solid",
        borderColor: selected ? "warning.main" : "divider",
        px: 1.5,
        py: 1,
        display: "flex",
        flexDirection: "column",
        gap: 0.5,
        transition: "border-color 0.15s",
        "&:hover": { borderColor: selected ? "warning.main" : "text.secondary" },
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
        <Typography variant="body2" fontWeight={700} noWrap title={label} sx={{ minWidth: 0 }}>
          {label}
        </Typography>
        {rollup.overdueCount > 0 && (
          <Typography variant="caption" fontWeight={700} sx={{ color: DUE_OVERDUE, flexShrink: 0 }}>
            {rollup.overdueCount} overdue
          </Typography>
        )}
      </Box>
      <Typography variant="caption" color="text.secondary">
        {rollup.complete}/{rollup.total} · {Math.round(rollup.completionPercent)}%
      </Typography>
      <LinearProgress
        variant="determinate"
        value={Math.min(rollup.completionPercent, 100)}
        sx={{
          height: 6, borderRadius: 3, bgcolor: "#E0E0E0",
          "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
          "& .MuiLinearProgress-bar": { bgcolor: color, borderRadius: 3 },
        }}
      />
      <Typography variant="caption" color="text.secondary">{endDateLabel}</Typography>
    </Box>
  );
}

interface AuditStripProps {
  framework: FrameworkRollup;
  /** null selects the "All audits" rollup. */
  selectedAuditId: number | null;
  onSelect: (auditId: number | null) => void;
}

export default function AuditStrip({ framework, selectedAuditId, onSelect }: AuditStripProps): JSX.Element {
  return (
    <Box role="tablist" sx={{ display: "flex", gap: 1.5, overflowX: "auto", pb: 0.5 }}>
      <AuditChip
        label="All audits"
        endDateLabel={formatDaysLeft(framework.daysLeft, "to nearest deadline")}
        rollup={framework}
        selected={selectedAuditId === null}
        onSelect={() => onSelect(null)}
      />
      {framework.audits.map((audit) => (
        <AuditChip
          key={audit.id}
          label={audit.name}
          endDateLabel={`Ends ${audit.deadline}`}
          rollup={audit}
          selected={selectedAuditId === audit.id}
          onSelect={() => onSelect(audit.id)}
        />
      ))}
    </Box>
  );
}

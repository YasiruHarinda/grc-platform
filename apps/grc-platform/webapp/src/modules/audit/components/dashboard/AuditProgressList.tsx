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

import { Box, Chip, LinearProgress, Typography } from "@wso2/oxygen-ui";
import { AlertTriangle } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { useNavigate } from "react-router";
import type { Audit } from "@modules/audit/types/audit";
import { DUE_OVERDUE } from "./dueDate";

// Per-audit completion rows built from the audits list controlCounts —
// no extra API needed (reuses GET /api/v1/audits).
export default function AuditProgressList({ audits }: { audits: Audit[] }): JSX.Element {
  const navigate = useNavigate();
  // Attention first: most overdue controls, then lowest completion.
  const active = audits
    .filter((a) => a.status === "ACTIVE")
    .sort((a, b) => {
      if (b.controlCounts.overdue !== a.controlCounts.overdue) {
        return b.controlCounts.overdue - a.controlCounts.overdue;
      }
      const pctA = a.controlCounts.total > 0 ? a.controlCounts.approved / a.controlCounts.total : 1;
      const pctB = b.controlCounts.total > 0 ? b.controlCounts.approved / b.controlCounts.total : 1;
      return pctA - pctB;
    });

  if (active.length === 0) {
    return (
      <Box sx={{ height: 240, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">No active audits</Typography>
      </Box>
    );
  }

  return (
    // flex-basis 0: the list fills the height set by the tallest sibling card
    // (it never stretches the row itself) and scrolls beyond that.
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.75, flex: "1 1 0", minHeight: 240, overflowY: "auto", pr: 0.5 }}>
      {active.map((audit) => {
        const { total, approved, overdue } = audit.controlCounts;
        const pct = total > 0 ? (approved / total) * 100 : 0;
        return (
          <Box
            key={audit.id}
            role="button"
            tabIndex={0}
            onClick={() => void navigate(`/audit/audits/${audit.id}`)}
            onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); void navigate(`/audit/audits/${audit.id}`); } }}
            sx={{
              cursor: "pointer",
              borderRadius: 1.5,
              px: 1, py: 0.75, mx: -1,
              transition: "background 0.15s",
              "&:hover": { bgcolor: "action.hover" },
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
              <Typography variant="body2" fontWeight={600} noWrap title={audit.name} sx={{ flex: 1 }}>
                {audit.name}
              </Typography>
              <Chip
                label={audit.framework.name}
                size="small"
                variant="outlined"
                sx={{ height: 20, fontSize: "0.65rem", flexShrink: 0 }}
              />
              {overdue > 0 && (
                <Chip
                  icon={<AlertTriangle size={12} />}
                  label={overdue}
                  size="small"
                  sx={{
                    height: 20, fontSize: "0.65rem", fontWeight: 700, flexShrink: 0,
                    color: DUE_OVERDUE, bgcolor: "transparent", border: `1px solid ${DUE_OVERDUE}`,
                    "& .MuiChip-icon": { color: DUE_OVERDUE },
                  }}
                />
              )}
              <Typography variant="body2" color="text.secondary" sx={{ textAlign: "right", flexShrink: 0, whiteSpace: "nowrap" }}>
                {approved}/{total} · {Math.round(pct)}%
              </Typography>
            </Box>
            <LinearProgress
              variant="determinate"
              value={pct}
              sx={{
                height: 8, borderRadius: 4, bgcolor: "#E0E0E0",
                "[data-color-scheme='dark'] &": { bgcolor: "rgba(255,255,255,0.12)" },
                "& .MuiLinearProgress-bar": { borderRadius: 4 },
              }}
            />
          </Box>
        );
      })}
    </Box>
  );
}

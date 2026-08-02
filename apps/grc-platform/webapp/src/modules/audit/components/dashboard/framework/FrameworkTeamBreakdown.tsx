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

import { Box, Typography, useTheme } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { ScopeRollup } from "@modules/audit/types/framework";
import { DUE_OVERDUE, DUE_SOON } from "@modules/audit/components/dashboard/dueDate";
import PhaseBar from "./PhaseBar";

interface FrameworkTeamBreakdownProps {
  scope: ScopeRollup;
}

// "Who owns the gap" — per-team completion, phase bar, and the assignee
// holding the most overdue work. The reader draws the escalation conclusion;
// this panel only states the fact it would be built on (§10).
export default function FrameworkTeamBreakdown({ scope }: FrameworkTeamBreakdownProps): JSX.Element {
  const theme = useTheme();

  if (scope.teams.length === 0) {
    return (
      <Box sx={{ height: 200, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">No team data</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, height: "100%", overflowY: "auto" }}>
      {scope.teams.map((team) => {
        const pct = team.total > 0 ? (team.completed / team.total) * 100 : 0;
        const completeColor = pct >= 100 ? theme.palette.success.main : theme.palette.primary.main;
        return (
          <Box key={team.team} sx={{ borderRadius: 1.5, px: 1, py: 0.75, "&:hover": { bgcolor: "action.hover" } }}>
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
              <Typography variant="body2" fontWeight={700} noWrap title={team.team} sx={{ mr: 1 }}>
                {team.team}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>
                {team.completed}/{team.total} · {Math.round(pct)}%
              </Typography>
            </Box>

            <PhaseBar counts={team.phaseCounts} completeColor={completeColor} height={6} />

            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mt: 0.5, flexWrap: "wrap", gap: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
                {team.openCount} open
                {team.overdueCount > 0 && (
                  <Typography component="span" variant="caption" fontWeight={700} sx={{ color: DUE_OVERDUE, ml: 1 }}>
                    {team.overdueCount} overdue
                  </Typography>
                )}
                {team.dueSoonCount > 0 && (
                  <Typography component="span" variant="caption" fontWeight={700} sx={{ color: DUE_SOON, ml: 1 }}>
                    {team.dueSoonCount} due soon
                  </Typography>
                )}
              </Typography>
              {team.topAssignee && (
                <Typography variant="caption" fontWeight={600}>
                  {team.topAssignee.name} ({team.topAssignee.overdueCount})
                  {team.additionalAssigneeCount > 0 && ` +${team.additionalAssigneeCount}`}
                </Typography>
              )}
            </Box>
          </Box>
        );
      })}
    </Box>
  );
}

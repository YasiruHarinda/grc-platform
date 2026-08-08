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

import { Box, Chip, Divider, Paper, Typography } from "@wso2/oxygen-ui";
import { AlertTriangle, CalendarClock, Inbox } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import CompletionRing from "./CompletionRing";
import { DUE_OVERDUE, DUE_SOON } from "./dueDate";

interface HeroBandProps {
  userName: string | null;
  completionPercent: number;
  activeAudits: number;
  totalControls: number;
  overdueCount: number;
  dueSoonCount: number;
  /** Count of items awaiting the current user; null hides the chip (e.g. management). */
  awaitingCount: number | null;
  awaitingLabel: string;
  onOverdueClick: () => void;
  onQueueClick: () => void;
}

function greeting(): string {
  const h = new Date().getHours();
  if (h < 12) return "Good morning";
  if (h < 17) return "Good afternoon";
  return "Good evening";
}

export default function HeroBand({
  userName,
  completionPercent,
  activeAudits,
  totalControls,
  overdueCount,
  dueSoonCount,
  awaitingCount,
  awaitingLabel,
  onOverdueClick,
  onQueueClick,
}: HeroBandProps): JSX.Element {
  const attention = overdueCount > 0 || dueSoonCount > 0 || (awaitingCount ?? 0) > 0;

  return (
    <Paper
      variant="outlined"
      sx={{
        borderRadius: 2,
        p: 3,
        display: "flex",
        alignItems: "center",
        gap: 3,
        flexWrap: "wrap",
      }}
    >
      <CompletionRing percent={completionPercent} />

      <Box sx={{ flex: 1, minWidth: 260 }}>
        <Typography variant="h4" fontWeight={700}>
          {greeting()}{userName ? `, ${userName}` : ""}
        </Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mt: 0.75, flexWrap: "wrap" }}>
          <Typography variant="body1" color="text.secondary">
            <Box component="span" fontWeight={700} color="text.primary">{activeAudits}</Box> active audit{activeAudits === 1 ? "" : "s"}
          </Typography>
          <Divider orientation="vertical" flexItem />
          <Typography variant="body1" color="text.secondary">
            <Box component="span" fontWeight={700} color="text.primary">{totalControls}</Box> controls in scope
          </Typography>
        </Box>

        {/* Attention chips */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 1.5, flexWrap: "wrap" }}>
          {!attention && (
            <Typography variant="body2" color="text.secondary">
              Nothing needs your attention right now.
            </Typography>
          )}
          {overdueCount > 0 && (
            <Chip
              clickable
              size="small"
              icon={<AlertTriangle size={14} />}
              label={`${overdueCount} overdue`}
              onClick={onOverdueClick}
              sx={{
                color: DUE_OVERDUE, fontWeight: 600,
                bgcolor: "transparent", border: `1px solid ${DUE_OVERDUE}`,
                "& .MuiChip-icon": { color: DUE_OVERDUE },
              }}
            />
          )}
          {(awaitingCount ?? 0) > 0 && (
            <Chip
              clickable
              size="small"
              icon={<Inbox size={14} />}
              label={`${awaitingCount} ${awaitingLabel}`}
              onClick={onQueueClick}
              sx={{
                color: DUE_SOON, fontWeight: 600,
                bgcolor: "transparent", border: `1px solid ${DUE_SOON}`,
                "& .MuiChip-icon": { color: DUE_SOON },
              }}
            />
          )}
          {dueSoonCount > 0 && (
            <Chip
              clickable
              size="small"
              icon={<CalendarClock size={14} />}
              label={`${dueSoonCount} due this week`}
              onClick={onQueueClick}
              sx={{
                color: "#3B82F6", fontWeight: 600,
                bgcolor: "transparent", border: "1px solid #3B82F6",
                "& .MuiChip-icon": { color: "#3B82F6" },
              }}
            />
          )}
        </Box>
      </Box>
    </Paper>
  );
}

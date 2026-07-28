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

import { Box, Chip, Paper, Stack, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RegisterAnalytics, RiskScore } from "../../api/riskApi";
import { darkCardSx } from "../cardStyles";
import { LEVEL_FALLBACK_COLORS, LEVEL_LABELS, LEVEL_ORDER } from "./constants";
import RegisterStatusChart from "./RegisterStatusChart";
import RiskHeatmap from "./RiskHeatmap";

interface RegisterSectionProps {
  register: RegisterAnalytics;
  scores: RiskScore[];
}

// One per-register dashboard section: residual heatmap on the left, the
// status × level stacked bar (with its own summary chips) on the right.
export default function RegisterSection({ register, scores }: RegisterSectionProps): JSX.Element {
  // Total Risks / High / Medium / Low chips are derived from status_levels
  // (open + closed) rather than a separate backend field, so they always
  // agree with what the chart's bars sum to.
  let totalCount = 0;
  const levelTotals = new Map<string, { count: number; color: string }>();
  for (const s of register.status_levels) {
    totalCount += s.count;
    const entry = levelTotals.get(s.risk_level) ?? { count: 0, color: s.color_code };
    entry.count += s.count;
    levelTotals.set(s.risk_level, entry);
  }

  return (
    <Paper variant="outlined" sx={{ p: 2.5, ...darkCardSx }}>
      <Typography variant="subtitle1" fontWeight={600} sx={{ mb: 2 }}>
        {register.register_name}
      </Typography>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "6fr 6fr" },
          gap: 3,
          alignItems: "start",
        }}
      >
        <RiskHeatmap cells={register.heatmap} scores={scores} />
        <Box>
          <Stack
            direction="row"
            alignItems="center"
            justifyContent="center"
            spacing={1.5}
            flexWrap="wrap"
            sx={{ mb: 1.5 }}
          >
            <Chip size="small" label={`${register.open_count} open`} variant="outlined" />
            <Chip size="small" label={`Total Risks: ${totalCount}`} variant="outlined" />
            {LEVEL_ORDER.filter((level) => levelTotals.has(level)).map((level) => {
              const { count, color } = levelTotals.get(level)!;
              const c = color || LEVEL_FALLBACK_COLORS[level];
              return (
                <Chip
                  key={level}
                  size="small"
                  label={`${LEVEL_LABELS[level] ?? level}: ${count}`}
                  sx={{ bgcolor: `${c}26`, border: `1px solid ${c}` }}
                />
              );
            })}
          </Stack>
          <RegisterStatusChart data={register.status_levels} />
        </Box>
      </Box>
    </Paper>
  );
}

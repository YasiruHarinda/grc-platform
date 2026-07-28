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

import { Box, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RiskStatusSummary } from "../../api/riskApi";
import { darkCardSx } from "../cardStyles";
import { CLOSED_COLOR, OPEN_COLOR } from "./constants";

interface SummaryCardsProps {
  summary: RiskStatusSummary;
}

interface StatTileProps {
  count: number;
  pct?: number | null;
  label: string;
  color?: string;
}

function StatTile({ count, pct, label, color }: StatTileProps): JSX.Element {
  return (
    <Card variant="outlined" sx={{ height: "100%", ...darkCardSx }}>
      <CardContent sx={{ height: "100%", display: "flex", alignItems: "center" }}>
        <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, flexWrap: "wrap" }}>
          <Typography
            variant="h3"
            component="span"
            sx={{ fontWeight: 700, lineHeight: 1, color }}
          >
            {count}
          </Typography>
          {pct != null && (
            <Typography variant="body2" component="span" color="text.secondary">
              ({pct}%)
            </Typography>
          )}
          <Typography variant="body2" component="span" color="text.secondary">
            {label}
          </Typography>
        </Box>
      </CardContent>
    </Card>
  );
}

// The Total / Open / Closed stat tiles at the top of the dashboard.
export default function SummaryCards({ summary }: SummaryCardsProps): JSX.Element {
  const pct = (n: number): number | null =>
    summary.total > 0 ? Math.round((n / summary.total) * 100) : null;

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(4, 1fr)" },
        gap: 2,
      }}
    >
      <StatTile count={summary.total}   label="Total Risks" />
      <StatTile count={summary.open}    pct={pct(summary.open)}   label="Open"   color={OPEN_COLOR} />
      <StatTile count={summary.closed}  pct={pct(summary.closed)} label="Closed" color={CLOSED_COLOR} />
      <StatTile count={summary.overdue} pct={pct(summary.overdue)} label="Overdue" />
    </Box>
  );
}

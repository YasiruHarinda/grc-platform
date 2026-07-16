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

import { BarChart, Line, YAxis } from "@wso2/oxygen-ui-charts-react";
import { Box, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { TrendPoint } from "../../api/riskApi";
import { formatMonthYear } from "../dashboard/constants";
import { AVG_SCORE_COLOR, CLOSED_TREND_COLOR, IDENTIFIED_COLOR } from "./constants";

interface TrendChartProps {
  data: TrendPoint[];
}

const CHART_HEIGHT = 320;

// "Risk Trend Over Time": grouped bars for risks identified vs. closed per
// month, with the average effective score overlaid as a line on a secondary
// (right) axis fixed to the 1–9 score range.
export default function TrendChart({ data }: TrendChartProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No risk activity in the last 12 months.
      </Typography>
    );
  }

  const rows = data.map((d) => ({
    month: formatMonthYear(d.month),
    identified: d.identified_count,
    closed: d.closed_count,
    avg_score: d.avg_score,
  }));

  return (
    <Box sx={{ display: "flex", alignItems: "stretch", gap: 0.5 }}>
      {/* Custom axis title, centered on the whole chart height (bars + legend)
          rather than recharts' internal "insideLeft" label, which gets
          clipped when the left margin is too tight to fit it. */}
      <Box
        sx={{
          height: CHART_HEIGHT,
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Typography
          variant="caption"
          color="text.secondary"
          noWrap
          sx={{ writingMode: "vertical-rl", transform: "rotate(180deg)" }}
        >
          Risks
        </Typography>
      </Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <BarChart
          data={rows}
          xAxisDataKey="month"
          height={CHART_HEIGHT}
          maxBarSize={28}
          isAnimationActive={false}
          margin={{ top: 8, right: 16, left: 8, bottom: 0 }}
          yAxis={{ show: true }}
          legend={{ show: true, align: "center", verticalAlign: "bottom" }}
          bars={[
            { dataKey: "identified", name: "Identified", fill: IDENTIFIED_COLOR, radius: [3, 3, 0, 0] },
            { dataKey: "closed", name: "Closed", fill: CLOSED_TREND_COLOR, radius: [3, 3, 0, 0] },
          ]}
        >
          <YAxis
            yAxisId="score"
            orientation="right"
            domain={[0, 9]}
            allowDecimals={false}
            tick={{ fontSize: 12 }}
            label={{ value: "Avg. Score", angle: 90, position: "insideRight", fontSize: 12 }}
          />
          <Line
            yAxisId="score"
            dataKey="avg_score"
            name="Avg. Effective Score"
            type="monotone"
            stroke={AVG_SCORE_COLOR}
            strokeWidth={2}
            dot={{ r: 3 }}
            connectNulls
            isAnimationActive={false}
          />
        </BarChart>
      </Box>
    </Box>
  );
}

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

import { LineChart } from "@wso2/oxygen-ui-charts-react";
import { Box, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { MonthRegisterCount } from "../../api/riskApi";
import { buildRegisterColorMap, formatMonthYear } from "../dashboard/constants";

interface RegisterTrendChartProps {
  data: MonthRegisterCount[];
  emptyMessage: string;
}

const CHART_HEIGHT = 320;

// One line per source register, x = month. Shared by the "Risks Identified
// by Source Register" and "Risks Closed by Source Register" charts — only
// their data and empty-state message differ. Only registers with at least
// one event in the trailing 12-month window get a line (backend-decided);
// once a register has one, every month is present so its line spans the
// full window.
export default function RegisterTrendChart({ data, emptyMessage }: RegisterTrendChartProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        {emptyMessage}
      </Typography>
    );
  }

  const registerColors = buildRegisterColorMap(data.map((d) => d.register_name));
  const rows = new Map<string, Record<string, string | number>>();
  for (const d of data) {
    if (!rows.has(d.month)) rows.set(d.month, { month: formatMonthYear(d.month) });
    rows.get(d.month)![d.register_name] = d.count;
  }

  const lines = [...registerColors.entries()].map(([register, color]) => ({
    dataKey: register,
    name: register,
    stroke: color,
    strokeWidth: 2,
    dot: { r: 3 },
    connectNulls: true,
    isAnimationActive: false,
  }));

  return (
    <Box sx={{ display: "flex", alignItems: "stretch", gap: 0.5 }}>
      {/* Custom axis title, centered on the whole chart height (line plot +
          legend), matching TrendChart's pattern. */}
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
          Number of Risks
        </Typography>
      </Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <LineChart
          data={[...rows.values()]}
          xAxisDataKey="month"
          lines={lines}
          height={CHART_HEIGHT}
          isAnimationActive={false}
          margin={{ top: 8, right: 16, left: 8, bottom: 0 }}
          yAxis={{ show: true }}
          legend={{ show: true, align: "center", verticalAlign: "bottom" }}
        />
      </Box>
    </Box>
  );
}

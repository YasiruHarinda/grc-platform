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

import { BarChart } from "@wso2/oxygen-ui-charts-react";
import { Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { MonthLevelCount } from "../../api/riskApi";
import { LEVEL_FALLBACK_COLORS, LEVEL_LABELS, LEVEL_ORDER, formatMonthYear } from "../dashboard/constants";

interface LevelDistributionChartProps {
  data: MonthLevelCount[];
}

const CHART_HEIGHT = 320;

// "Risk Level Distribution Over Time": risks identified per month, stacked
// by effective severity — shows how the severity mix shifts, distinct from
// TrendChart's raw identified/closed volume.
export default function LevelDistributionChart({ data }: LevelDistributionChartProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No risk activity in the last 12 months.
      </Typography>
    );
  }

  const rows = new Map<string, Record<string, string | number>>();
  const order: string[] = [];
  for (const d of data) {
    const label = formatMonthYear(d.month);
    if (!rows.has(label)) {
      rows.set(label, { month: label });
      order.push(label);
    }
    if (d.count > 0) rows.get(label)![d.risk_level] = d.count;
  }

  const bars = LEVEL_ORDER.map((level) => ({
    dataKey: level,
    name: LEVEL_LABELS[level],
    fill: data.find((d) => d.risk_level === level)?.color_code || LEVEL_FALLBACK_COLORS[level],
    stackId: "level",
  }));

  return (
    <BarChart
      data={order.map((label) => rows.get(label)!)}
      xAxisDataKey="month"
      bars={bars}
      height={CHART_HEIGHT}
      maxBarSize={28}
      isAnimationActive={false}
      margin={{ top: 8, right: 16, left: 0, bottom: 0 }}
      yAxis={{ show: true }}
      legend={{ show: true, align: "center", verticalAlign: "bottom" }}
    />
  );
}

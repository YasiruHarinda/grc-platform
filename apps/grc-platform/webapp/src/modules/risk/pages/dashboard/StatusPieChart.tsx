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

import { PieChart } from "@wso2/oxygen-ui-charts-react";
import { Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RiskStatusSummary } from "../../api/riskApi";
import { CLOSED_COLOR, OPEN_COLOR } from "./constants";

interface StatusPieChartProps {
  summary: RiskStatusSummary;
}

// Overall risk status distribution: open vs. closed share of all risks.
export default function StatusPieChart({ summary }: StatusPieChartProps): JSX.Element {
  if (summary.total === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No risks recorded yet.
      </Typography>
    );
  }

  const data = [
    { name: "Open", value: summary.open },
    { name: "Closed", value: summary.closed },
  ];

  return (
    <PieChart
      data={data}
      height={320}
      colors={[OPEN_COLOR, CLOSED_COLOR]}
      isAnimationActive={false}
      pies={[
        {
          dataKey: "value",
          nameKey: "name",
          innerRadius: "50%",
          outerRadius: "85%",
          paddingAngle: 2,
          label: ({ percent }: { percent?: number }) =>
            `${((percent ?? 0) * 100).toFixed(1)}%`,
        },
      ]}
      legend={{ show: true, align: "center", verticalAlign: "bottom" }}
    />
  );
}

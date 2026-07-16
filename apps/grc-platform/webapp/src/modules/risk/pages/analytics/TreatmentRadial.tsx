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

import { RadialBarChart } from "@wso2/oxygen-ui-charts-react";
import { Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { TreatmentShare } from "../../api/riskApi";
import { TREATMENT_COLORS, TREATMENT_LABELS, TREATMENT_ORDER } from "../dashboard/constants";

interface TreatmentRadialProps {
  data: TreatmentShare[];
}

// Org-wide "Risk Treatment Strategies": total open risk count per strategy,
// one ring per strategy — a single organisation-wide read, distinct from the
// Dashboard's per-register stacked-bar breakdown of the same field.
export default function TreatmentRadial({ data }: TreatmentRadialProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open risks.
      </Typography>
    );
  }

  const byStrategy = new Map(data.map((d) => [d.treatment_strategy, d.count]));
  const rows = TREATMENT_ORDER.filter((strategy) => strategy !== "UNSPECIFIED").map((strategy) => ({
    name: TREATMENT_LABELS[strategy],
    count: byStrategy.get(strategy) ?? 0,
    fill: TREATMENT_COLORS[strategy],
  }));

  return (
    <RadialBarChart
      data={rows}
      height={320}
      innerRadius="20%"
      outerRadius="90%"
      startAngle={90}
      endAngle={-270}
      radialBars={[
        {
          dataKey: "count",
          background: { fill: "rgba(128,128,128,0.08)" },
          cornerRadius: 6,
          isAnimationActive: false,
        },
      ]}
      legend={{ show: true, align: "center", verticalAlign: "bottom" }}
      tooltip={{ show: true }}
    />
  );
}

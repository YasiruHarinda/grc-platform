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

import { CalendarClock, Gauge, TrendingUp } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import StatGrid, { type StatConfig } from "@components/stat-grid/StatGrid";
import type { AnalyticsKPIs } from "../../api/riskApi";

type KpiKey = "new_risks_this_month" | "avg_days_to_close" | "avg_effective_score";

const CONFIGS: StatConfig<KpiKey>[] = [
  { key: "new_risks_this_month", label: "New This Month", icon: TrendingUp, iconColor: "info" },
  { key: "avg_days_to_close", label: "Avg. Days to Close", icon: CalendarClock, iconColor: "success", tooltipText: "Average time from identification to closure, across closed risks" },
  { key: "avg_effective_score", label: "Avg. Risk Score", icon: Gauge, iconColor: "secondary", tooltipText: "Average effective residual score across open risks (1–9)" },
];

interface KpiTilesProps {
  loading: boolean;
  kpis: AnalyticsKPIs | null;
}

// The three "Key Risk Metrics" tiles at the top of the Analytics page.
// Total/Open/Overdue counts live on the Dashboard's summary cards instead.
export default function KpiTiles({ loading, kpis }: KpiTilesProps): JSX.Element {
  const stats: Partial<Record<KpiKey, number>> = kpis
    ? {
        new_risks_this_month: kpis.new_risks_this_month,
        ...(kpis.avg_days_to_close != null ? { avg_days_to_close: kpis.avg_days_to_close } : {}),
        ...(kpis.avg_effective_score != null ? { avg_effective_score: kpis.avg_effective_score } : {}),
      }
    : {};

  return (
    <StatGrid<KpiKey>
      isLoading={loading}
      configs={CONFIGS}
      stats={stats}
      entityName="risk analytics"
      itemSize={{ xs: 12, sm: 6, md: 4 }}
      valueFormatter={(v) => (Number.isInteger(v) ? v : v.toFixed(1))}
    />
  );
}

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

import { Button } from "@wso2/oxygen-ui";
import { Download } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import type { AnalyticsSummary } from "../../api/riskApi";

interface CsvExportButtonProps {
  data: AnalyticsSummary | null;
  registerLabel: string;
}

function escapeCsv(value: unknown): string {
  const raw = value == null ? "" : String(value);
  // Neutralize formula injection: a leading =, +, -, @, tab, or CR is
  // interpreted as a formula by Excel/Sheets when the CSV is opened.
  const s = /^[=+\-@\t\r]/.test(raw) ? `'${raw}` : raw;
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

function section<T extends object>(title: string, rows: T[]): string {
  if (rows.length === 0) return `${title}\n(no data)\n`;
  const headers = Object.keys(rows[0]) as (keyof T)[];
  const lines = [
    title,
    headers.join(","),
    ...rows.map((r) => headers.map((h) => escapeCsv(r[h])).join(",")),
  ];
  return lines.join("\n") + "\n";
}

function buildCsv(data: AnalyticsSummary): string {
  const parts = [
    section("Key Risk Metrics", [
      { metric: "New Risks This Month", value: data.kpis.new_risks_this_month },
      { metric: "Avg. Days to Close", value: data.kpis.avg_days_to_close ?? "" },
      { metric: "Avg. Effective Score", value: data.kpis.avg_effective_score ?? "" },
    ]),
    section("Risk Trend Over Time", data.trend),
    section("Risk Level Distribution Over Time", data.level_distribution),
    section("Risks by Register", data.register_shares ?? []),
    section("Compliance Reference Distribution", data.compliance_distribution),
    section("Risk Treatment Strategies", data.treatment_mix),
    section("Workflow Status Funnel", data.workflow_funnel),
    section("Aging Open Risks", data.aging_risks),
  ];
  return parts.join("\n");
}

// Exports the currently loaded (and register-filtered) analytics payload as
// a multi-section CSV — client-side only, no extra backend endpoint.
export default function CsvExportButton({ data, registerLabel }: CsvExportButtonProps): JSX.Element {
  const handleExport = (): void => {
    if (!data) return;
    const csv = buildCsv(data);
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    const stamp = new Date().toISOString().slice(0, 10);
    a.href = url;
    a.download = `risk-analytics-${registerLabel}-${stamp}.csv`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  };

  return (
    <Button
      variant="outlined"
      size="small"
      startIcon={<Download size={16} />}
      disabled={!data}
      onClick={handleExport}
    >
      Export CSV
    </Button>
  );
}

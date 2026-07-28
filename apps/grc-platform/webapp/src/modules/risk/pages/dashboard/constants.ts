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

import { parseDateStr } from "../risk-registers/utils";

// Chart palette for the risk dashboard. Categorical hues are assigned in fixed
// order (never cycled) and were validated for colorblind-safe adjacent-pair
// separation. Segment labels stay visible because two hues sit below 3:1
// contrast on light surfaces.

export const TREATMENT_ORDER = ["REMEDIATE", "ACCEPT", "TRANSFER", "VOID", "UNSPECIFIED"] as const;

export const TREATMENT_LABELS: Record<string, string> = {
  REMEDIATE: "To be Remediated",
  ACCEPT: "Risk Accepted",
  TRANSFER: "Transfer",
  VOID: "Avoid",
  UNSPECIFIED: "Unspecified",
};

export const TREATMENT_COLORS: Record<string, string> = {
  REMEDIATE: "#2a78d6",
  ACCEPT: "#1baf7a",
  TRANSFER: "#eda100",
  VOID: "#4a3aa7",
  UNSPECIFIED: "#8a8a8a",
};

// Open/closed pair for the status pie and status chips (open = attention red).
export const OPEN_COLOR = "#e34948";
export const CLOSED_COLOR = "#2a78d6";

// Fixed-order categorical slots for compliance certifications, assigned to
// cert names alphabetically. Certs beyond the 8 slots share the neutral color.
const CERT_PALETTE = [
  "#2a78d6",
  "#1baf7a",
  "#eda100",
  "#008300",
  "#4a3aa7",
  "#e34948",
  "#e87ba4",
  "#eb6834",
];
const CERT_OVERFLOW_COLOR = "#6b7280";

export const LEVEL_ORDER = ["HIGH", "MEDIUM", "LOW"] as const;

export const LEVEL_LABELS: Record<string, string> = {
  HIGH: "High",
  MEDIUM: "Medium",
  LOW: "Low",
};

// X-axis order for each register's status chart: closed risks, then every
// open-risk treatment strategy.
export const STATUS_BUCKET_ORDER = ["CLOSED", "REMEDIATE", "ACCEPT", "TRANSFER", "VOID"] as const;

export const STATUS_BUCKET_LABELS: Record<string, string> = {
  CLOSED: "Closed",
  REMEDIATE: "To be Remediated",
  ACCEPT: "Accepted",
  TRANSFER: "Transfer",
  VOID: "Avoid",
};

// Severity palette shared across the risk module: dashboard charts use it as
// a fallback when a payload row is missing its DB color_code, and
// EditRiskDialog uses it directly to color the live score preview.
export const LEVEL_FALLBACK_COLORS: Record<string, string> = {
  HIGH: "#FF0000",
  MEDIUM: "#FF9900",
  LOW: "#00B050",
};

// Recharts stores a stacked bar segment's value as its cumulative [start, end]
// range, and LabelList's default valueAccessor extracts the range END — so
// labels show running totals. This accessor recovers the segment's own value;
// pass it as `valueAccessor` on a stacked bar's label config.
export function stackedSegmentAccessor(entry: { value: number | [number, number] }): number {
  return Array.isArray(entry.value) ? entry.value[1] - entry.value[0] : entry.value;
}

// Picks a readable label color for text drawn on top of a colored segment.
export function labelColorOn(hex: string): string {
  const n = parseInt(hex.replace("#", ""), 16);
  if (Number.isNaN(n)) return "#ffffff";
  const r = (n >> 16) & 0xff;
  const g = (n >> 8) & 0xff;
  const b = n & 0xff;
  // Relative luminance approximation; light segments get dark ink.
  return 0.299 * r + 0.587 * g + 0.114 * b > 150 ? "#1a1a19" : "#ffffff";
}

// Maps cert names to palette slots alphabetically so a cert keeps its color
// across every register and across reloads.
export function buildCertColorMap(certNames: string[]): Map<string, string> {
  const sorted = [...new Set(certNames)].sort((a, b) => a.localeCompare(b));
  const map = new Map<string, string>();
  sorted.forEach((name, i) => {
    map.set(name, i < CERT_PALETTE.length ? CERT_PALETTE[i] : CERT_OVERFLOW_COLOR);
  });
  return map;
}

// Maps register names to the same categorical palette, alphabetically, so a
// register keeps a stable color across the by-register trend charts and
// across reloads — mirrors buildCertColorMap's assignment rule.
export function buildRegisterColorMap(registerNames: string[]): Map<string, string> {
  const sorted = [...new Set(registerNames)].sort((a, b) => a.localeCompare(b));
  const map = new Map<string, string>();
  sorted.forEach((name, i) => {
    map.set(name, i < CERT_PALETTE.length ? CERT_PALETTE[i] : CERT_OVERFLOW_COLOR);
  });
  return map;
}

// Builds the cert-distribution chart's subtitle from whichever certification
// names are actually present in the current data, so newly added certs show
// up automatically instead of needing a hardcoded list.
export function certListSentence(certNames: string[]): string {
  const unique = [...new Set(certNames)].sort((a, b) => a.localeCompare(b));
  if (unique.length === 0) return "";
  const list = new Intl.ListFormat("en", { style: "long", type: "conjunction" }).format(unique);
  return `Open risks mapped to compliance certifications: ${list}`;
}

export function formatMonthYear(value: string | null): string {
  if (!value) return "";
  const d = parseDateStr(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleDateString("en-US", { year: "numeric", month: "short" });
}

// Renders the treatment column shown in tables: "Accept", or
// "Remediate (by Jul 2026)" when an implementation date exists.
export function formatTreatment(
  strategy: string | null,
  implementationDate: string | null,
): string {
  if (!strategy) return "—";
  const label = TREATMENT_LABELS[strategy] ?? strategy;
  if (strategy === "REMEDIATE" && implementationDate) {
    return `${label} (by ${formatMonthYear(implementationDate)})`;
  }
  return label;
}

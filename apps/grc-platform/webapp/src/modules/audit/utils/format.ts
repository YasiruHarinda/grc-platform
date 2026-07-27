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

// Parses a "YYYY-MM-DD" string as a local calendar date (not UTC).
function parseLocalDate(dateStr: string): Date {
  const parts = dateStr.split("-");
  return new Date(Number(parts[0]), Number(parts[1]) - 1, Number(parts[2]));
}

/** Formats "YYYY-MM-DD" → "Jun 22, 2026" */
export function formatAuditDate(dateStr: string): string {
  return parseLocalDate(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

/**
 * Days from today until dateStr ("YYYY-MM-DD"); negative when past.
 * Returns null for empty input.
 */
export function daysLeft(dateStr: string | null | undefined): number | null {
  if (!dateStr) return null;
  const target = parseLocalDate(dateStr);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return Math.round((target.getTime() - today.getTime()) / 86_400_000);
}

/**
 * Compact relative time from an ISO timestamp → "just now", "5m ago", "3h ago",
 * "2d ago", falling back to "Jun 22" for anything older than a week. Returns ""
 * for invalid input.
 */
export function relativeTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const secs = Math.round((Date.now() - d.getTime()) / 1000);
  // A timestamp meaningfully in the future is a data problem (usually a server
  // time-zone mismatch), not a real event. Show the absolute time so it reads as
  // odd rather than silently collapsing every such row to "just now". A minute of
  // clock skew is tolerated as "just now".
  if (secs < -60) return formatTimestamp(iso);
  if (secs < 45) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

/** Formats a full ISO timestamp → "Jun 22, 2026 02:30 PM". Returns "" for invalid input. */
export function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: true,
  });
}

/** Formats a date range. Same year omits the year from the start: "Jan 1 – Dec 31, 2026" */
export function formatDateRange(start: string, end: string): string {
  const s = parseLocalDate(start);
  const e = parseLocalDate(end);
  const sameYear = s.getFullYear() === e.getFullYear();
  const startStr = s.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    ...(sameYear ? {} : { year: "numeric" }),
  });
  const endStr = e.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
  return `${startStr} – ${endStr}`;
}

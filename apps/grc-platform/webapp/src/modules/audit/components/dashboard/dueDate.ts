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

export const DUE_OVERDUE = "#DC2626";
export const DUE_SOON = "#B45309";

/** Number of days ahead that counts as "due soon" on the dashboard. */
export const DUE_SOON_DAYS = 7;

export interface DueInfo {
  color: string;
  label: string;
  sortKey: number;
  /** Days until due; negative = overdue; +Infinity when no due date. */
  days: number;
}

// dueInfo derives a color, a relative label, and a sort key from a YYYY-MM-DD date.
//
// "Today" is anchored to the UTC calendar date, not the browser's local one —
// the DB connection is pinned to UTC (see normalizeDSN in entity/compliance-entity's
// db/mysql.go), so the Work Queue's Due Soon/Overdue buckets are computed against
// UTC's CURDATE(). For a user in a timezone ahead of UTC, local midnight arrives
// hours before UTC midnight; using local "today" here made a due date already
// bucketed as "Due Soon" by the backend render as "1d overdue" client-side.
export function dueInfo(dueDate: string | null | undefined): DueInfo {
  if (!dueDate) {
    return { color: "text.disabled", label: "—", sortKey: Number.POSITIVE_INFINITY, days: Number.POSITIVE_INFINITY };
  }
  const due = new Date(`${dueDate}T00:00:00Z`);
  const now = new Date();
  const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const days = Math.round((due.getTime() - today.getTime()) / 86_400_000);

  let color = "text.primary";
  let rel = "";
  if (days < 0) { color = DUE_OVERDUE; rel = `${-days}d overdue`; }
  else if (days === 0) { color = DUE_SOON; rel = "due today"; }
  else if (days <= 3) { color = DUE_SOON; rel = `in ${days}d`; }
  else { rel = `in ${days}d`; }

  return { color, label: `${dueDate} · ${rel}`, sortKey: days, days };
}

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

import type { Quarter, TreatmentStrategy } from "./types";

export const QUARTERS: { value: Quarter; label: string }[] = [
  { value: "Q1", label: "Q1 (Jan – Mar)" },
  { value: "Q2", label: "Q2 (Apr – Jun)" },
  { value: "Q3", label: "Q3 (Jul – Sep)" },
  { value: "Q4", label: "Q4 (Oct – Dec)" },
];

export const TREATMENT_STRATEGIES: { value: TreatmentStrategy; label: string }[] = [
  { value: "REMEDIATE", label: "Remediate" },
  { value: "ACCEPT",   label: "Accept" },
  { value: "TRANSFER", label: "Transfer" },
  { value: "VOID",     label: "Avoid" },
];

export const getCurrentYear = (): number => new Date().getFullYear();

export const getCurrentQuarter = (): Quarter => {
  const month = new Date().getMonth(); // 0-indexed
  if (month < 3)  return "Q1";
  if (month < 6)  return "Q2";
  if (month < 9)  return "Q3";
  return "Q4";
};

// Year range: 10 years back, up to and including the current year. Never a
// future year.
//
// This is the year the risk was IDENTIFIED — it becomes part of the risk code
// (YEAR-TEAM-QUARTER-NNNN) — so a future year cannot be true: nothing has been
// identified in a year that has not happened yet.
//
// The range reaches well back because a risk is registered for the year it was
// identified, not the year someone got around to typing it in. An audit or a
// review routinely surfaces something that has been true for years, and it
// belongs in the register under the year it dates from.
const YEARS_BACK = 10;

export const YEAR_OPTIONS: number[] = Array.from(
  { length: YEARS_BACK + 1 },
  (_, i) => getCurrentYear() - i,
);

// Produces the canonical risk code string shown in the UI and stored in the DB.
// Format: YEAR-TEAMCODE-QUARTER-NNNN  (e.g. 2026-ASG-Q2-0001)
export const buildRiskCode = (
  year: number,
  teamCode: string,
  quarter: string,
  sequenceId: number,
): string =>
  `${year}-${teamCode}-${quarter}-${String(sequenceId).padStart(4, "0")}`;

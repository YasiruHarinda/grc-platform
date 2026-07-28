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

// Year range: 3 years back to 2 years ahead
export const YEAR_OPTIONS: number[] = Array.from(
  { length: 6 },
  (_, i) => getCurrentYear() - 3 + i,
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

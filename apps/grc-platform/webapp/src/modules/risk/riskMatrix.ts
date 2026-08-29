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

// The axes of the 3x3 likelihood x impact risk-score matrix, shared by every
// place that renders it: the Add Risk form (RiskAssessmentStep), the Edit /
// Reassess dialogs (RiskScoreGrid), and the read-only Admin Console table
// (RiskScoresPage). Orientation: Likelihood Y-axis top (High 3) -> bottom
// (Low 1), Impact X-axis left (Minor 1) -> right (Major 3). Single-sourced so
// the axis order cannot drift between the interactive grids and the read-only
// one.
export const LIKELIHOOD_ROWS = [
  { value: 3, label: "High 3" },
  { value: 2, label: "Medium 2" },
  { value: 1, label: "Low 1" },
] as const;

export const IMPACT_COLS = [
  { value: 1, label: "Minor 1" },
  { value: 2, label: "Moderate 2" },
  { value: 3, label: "Major 3" },
] as const;

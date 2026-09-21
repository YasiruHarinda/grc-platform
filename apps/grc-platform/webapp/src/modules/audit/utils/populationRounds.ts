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

import type { PopulationFile, PopulationRound, PopulationView } from "@modules/audit/api/useGetPopulation";

export interface PopulationRoundEntry {
  round: PopulationRound;
  files: PopulationFile[];
  /** Only the current round can still be edited; earlier ones are history. */
  isCurrent: boolean;
}

/**
 * Every population round that holds something to show (files or a note),
 * oldest first with the current round last. A round with neither — a fresh
 * PENDING one, or one an external auditor was stripped of — is dropped, the
 * same way the evidence list drops empty rounds.
 */
export function populationRounds(view: PopulationView | undefined): PopulationRoundEntry[] {
  if (!view) return [];
  const rounds: PopulationRoundEntry[] = [
    ...(view.earlierRounds ?? []).map((e) => ({ round: e.round, files: e.populationFiles, isCurrent: false })),
    { round: view.round, files: view.populationFiles, isCurrent: true },
  ];
  return rounds.filter((r) => r.files.length > 0 || Boolean(r.round.attestation));
}

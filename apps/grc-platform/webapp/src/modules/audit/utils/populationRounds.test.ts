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

import { describe, expect, it } from "vitest";
import type { PopulationFile, PopulationRound, PopulationView } from "@modules/audit/api/useGetPopulation";
import { populationRounds } from "./populationRounds";

function round(id: number, status: PopulationRound["status"], attestation: string | null = null): PopulationRound {
  return {
    id, controlId: 1, status, referenceNumber: null, description: null, dueDate: null, comments: null,
    attestation, createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z",
  };
}

function file(id: number, populationId: number): PopulationFile {
  return {
    id, populationId, fileKind: "POPULATION", fileName: `f${id}.csv`, filePath: `p/f${id}.csv`,
    fileType: null, fileSize: null, createdBy: "u", createdByName: "U", createdAt: "2026-09-01T00:00:00Z", readUrl: null,
  };
}

function view(partial: Partial<PopulationView> & Pick<PopulationView, "round">): PopulationView {
  return { populationFiles: [], sampleFiles: [], sampleReference: null, ...partial };
}

describe("populationRounds", () => {
  it("returns nothing before the view has loaded", () => {
    expect(populationRounds(undefined)).toEqual([]);
  });

  it("lists earlier rounds oldest first with the current round last and flagged", () => {
    const result = populationRounds(view({
      round: round(2, "SUBMITTED"),
      populationFiles: [file(20, 2)],
      earlierRounds: [{ round: round(1, "COMPLIANCE_REJECTED"), populationFiles: [file(10, 1)] }],
    }));
    expect(result.map((r) => [r.round.id, r.isCurrent])).toEqual([[1, false], [2, true]]);
  });

  it("drops a round with neither files nor a note, including an empty current one", () => {
    const result = populationRounds(view({
      round: round(2, "PENDING"),
      earlierRounds: [{ round: round(1, "AUDITOR_REJECTED"), populationFiles: [file(10, 1)] }],
    }));
    expect(result.map((r) => r.round.id)).toEqual([1]);
    expect(result[0].isCurrent).toBe(false);
  });

  it("keeps a note-only round", () => {
    const result = populationRounds(view({ round: round(1, "SUBMITTED", "no in-scope items") }));
    expect(result).toHaveLength(1);
    expect(result[0].files).toEqual([]);
  });

  it("copes with a response that has no earlierRounds", () => {
    const result = populationRounds(view({ round: round(1, "SUBMITTED"), populationFiles: [file(10, 1)] }));
    expect(result.map((r) => r.round.id)).toEqual([1]);
  });
});

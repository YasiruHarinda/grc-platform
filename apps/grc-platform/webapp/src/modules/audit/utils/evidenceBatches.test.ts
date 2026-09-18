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
import type { EvidenceFile, EvidenceSubmission } from "@modules/audit/api/useGetEvidence";
import { groupIntoBatches } from "./evidenceBatches";

let nextId = 1;

function file(createdAt: string, createdBy = "uuid-alice", createdByName = "Alice"): EvidenceFile {
  return {
    id: nextId++,
    fileName: `f${nextId}.pdf`,
    fileType: null,
    fileSize: null,
    readUrl: null,
    createdBy,
    createdByName,
    createdAt,
  };
}

function round(files: EvidenceFile[]): EvidenceSubmission {
  return {
    id: 100,
    controlId: 5,
    status: "SUBMITTED",
    folderPath: null,
    files,
    createdBy: "uuid-alice",
    createdByName: "Alice",
    createdAt: "2026-09-01T10:00:00Z",
  };
}

describe("groupIntoBatches", () => {
  it("keeps one upload action together", () => {
    // The blobs are already uploaded by the time the rows are written, so a
    // batch's rows land within milliseconds of each other.
    const batches = groupIntoBatches(
      round([
        file("2026-09-01T10:00:00.100Z"),
        file("2026-09-01T10:00:00.240Z"),
        file("2026-09-01T10:00:00.390Z"),
      ]),
    );
    expect(batches).toHaveLength(1);
    expect(batches[0].files).toHaveLength(3);
    expect(batches[0].byName).toBe("Alice");
  });

  it("splits a later Add Files into its own batch", () => {
    const batches = groupIntoBatches(
      round([file("2026-09-01T10:00:00Z"), file("2026-09-01T10:04:00Z")]),
    );
    expect(batches).toHaveLength(2);
    expect(batches[0].at).toBe("2026-09-01T10:00:00Z");
    expect(batches[1].at).toBe("2026-09-01T10:04:00Z");
  });

  it("splits back-to-back actions a testing user would make", () => {
    // 40s apart: the case the first 2-minute window wrongly merged.
    const batches = groupIntoBatches(
      round([file("2026-09-01T10:00:00Z"), file("2026-09-01T10:00:40Z")]),
    );
    expect(batches).toHaveLength(2);
  });

  it("splits on a different uploader even within the gap", () => {
    const batches = groupIntoBatches(
      round([
        file("2026-09-01T10:00:00Z"),
        file("2026-09-01T10:00:02Z", "uuid-bob", "Bob"),
      ]),
    );
    expect(batches).toHaveLength(2);
    expect(batches[1].byName).toBe("Bob");
  });

  it("orders batches oldest first regardless of API order", () => {
    // The API returns a round's files newest-first.
    const batches = groupIntoBatches(
      round([file("2026-09-01T12:00:00Z"), file("2026-09-01T10:00:00Z")]),
    );
    expect(batches.map((b) => b.at)).toEqual([
      "2026-09-01T10:00:00Z",
      "2026-09-01T12:00:00Z",
    ]);
  });

  it("falls back to the raw uuid when no display name resolved", () => {
    const batches = groupIntoBatches(round([file("2026-09-01T10:00:00Z", "uuid-alice", "")]));
    expect(batches[0].byName).toBe("uuid-alice");
  });

  it("returns no batches for a fileless round", () => {
    expect(groupIntoBatches(round([]))).toEqual([]);
    expect(groupIntoBatches({ ...round([]), files: null })).toEqual([]);
  });
});

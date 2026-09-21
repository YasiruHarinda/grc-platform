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

import type { EvidenceFile, EvidenceSubmission } from "@modules/audit/api/useGetEvidence";

// One upload action ("Submit" or "Add Files") writes its file rows in a single
// backend call — the blobs are already uploaded by then, so the rows land
// back-to-back in well under a second even for a large batch. Nothing stores a
// batch id, so consecutive rows by the same uploader closer together than this
// are treated as one action. Kept tight on purpose: two deliberate actions
// minutes apart must not merge, and merging is the failure that loses the
// per-submission detail this grouping exists to show.
const BATCH_GAP_MS = 15_000;

/** The fields batching needs — shared by evidence and population/sample files. */
interface BatchableFile {
  id: number;
  createdBy: string;
  createdByName: string;
  createdAt: string;
}

/** One upload action within a round: who added these files, and when. */
export interface FileBatch<T extends BatchableFile = BatchableFile> {
  key: string;
  at: string;
  byName: string;
  files: T[];
}

function timeOf(iso: string): number | null {
  const t = new Date(iso).getTime();
  return Number.isNaN(t) ? null : t;
}

/**
 * Splits a round's files into the upload actions that produced them, oldest
 * first — a round stays open through internal review, so "Add Files" keeps
 * appending to it and the round's own submitter/timestamp stops describing
 * everything inside it. `keyPrefix` namespaces the batch keys (the round id).
 */
export function groupFilesIntoBatches<T extends BatchableFile>(
  input: T[],
  keyPrefix: string | number,
): FileBatch<T>[] {
  const files = [...input].sort(
    (a, b) => (timeOf(a.createdAt) ?? 0) - (timeOf(b.createdAt) ?? 0),
  );
  const batches: FileBatch<T>[] = [];
  for (const f of files) {
    const last = batches[batches.length - 1];
    if (last && last.files[0].createdBy === f.createdBy) {
      const startT = timeOf(last.at);
      const t = timeOf(f.createdAt);
      // Measured from the batch's first file, not the previous one, so a slow
      // trickle of files can't chain into a single long batch.
      // Unparseable timestamps can't split a batch — fall back to the uploader.
      const gap = startT !== null && t !== null ? t - startT : 0;
      if (gap <= BATCH_GAP_MS) {
        last.files.push(f);
        continue;
      }
    }
    batches.push({
      key: `${keyPrefix}-${f.id}`,
      at: f.createdAt,
      byName: f.createdByName || f.createdBy,
      files: [f],
    });
  }
  return batches;
}

export function groupIntoBatches(sub: EvidenceSubmission): FileBatch<EvidenceFile>[] {
  return groupFilesIntoBatches(sub.files ?? [], sub.id);
}

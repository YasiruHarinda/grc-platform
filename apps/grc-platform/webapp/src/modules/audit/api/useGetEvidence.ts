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

import { useQuery } from "@tanstack/react-query";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import { extractErrorMessage } from "@modules/audit/api/apiError";

export interface EvidenceFile {
  id: number;
  fileName: string;
  fileType: string | null;
  fileSize: number | null;
  readUrl: string | null; // short-lived read SAS URL for viewing/downloading
  createdBy: string;
  createdAt: string;
}

export interface EvidenceSubmission {
  id: number;
  controlId: number;
  status: string;
  folderPath: string | null;
  files: EvidenceFile[] | null; // null when a submission round has no files
  createdBy: string;
  createdAt: string;
}

export const evidenceQueryKey = (auditId: number, controlId: number) =>
  ["audit", "evidence", auditId, controlId] as const;

/** Fetches all evidence submissions (newest first) + their files for a control. */
export function useGetEvidence(auditId: number, controlId: number, enabled: boolean) {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: evidenceQueryKey(auditId, controlId),
    enabled,
    queryFn: async (): Promise<EvidenceSubmission[]> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/evidence`,
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to load evidence (${res.status})`));
      }
      return res.json() as Promise<EvidenceSubmission[]>;
    },
  });
}

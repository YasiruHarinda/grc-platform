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

export interface PopulationFile {
  id: number;
  populationId: number;
  fileKind: "POPULATION" | "SAMPLE";
  fileName: string;
  filePath: string;
  fileType: string | null;
  fileSize: number | null;
  createdAt: string;
  readUrl: string | null;
}

export interface PopulationRound {
  id: number;
  controlId: number;
  status: "PENDING" | "SUBMITTED" | "COMPLIANCE_APPROVED" | "COMPLIANCE_REJECTED" | "APPROVED" | "AUDITOR_REJECTED";
  referenceNumber: number | null;
  description: string | null;
  dueDate: string | null;
  comments: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface PopulationView {
  round: PopulationRound;
  populationFiles: PopulationFile[];
  sampleFiles: PopulationFile[];
  sampleReference: string | null;
}

export const populationQueryKey = (auditId: number, controlId: number) =>
  ["audit", "population", auditId, controlId] as const;

/** Fetches the control's current population round: its files (split population/sample) and the auditor's sample note. */
export function useGetPopulation(auditId: number, controlId: number, enabled: boolean) {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: populationQueryKey(auditId, controlId),
    enabled,
    queryFn: async (): Promise<PopulationView> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/population`,
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to load population (${res.status})`));
      }
      return res.json() as Promise<PopulationView>;
    },
  });
}

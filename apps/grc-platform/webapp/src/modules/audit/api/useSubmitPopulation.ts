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

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import { controlsQueryKey } from "@modules/audit/api/useGetControls";
import { populationQueryKey } from "@modules/audit/api/useGetPopulation";
import { extractErrorMessage } from "@modules/audit/api/apiError";

interface SubmitPopulationPayload {
  auditId: number;
  controlId: number;
  files: File[];
  // Written note standing in for population files — required when files is
  // empty (files, a note, or both; at least one). Unlike evidence's
  // attestation, there's no privilege gate: anyone who can submit population
  // files can use this too (mirrors sample selection's own files-or-note rule).
  attestation?: string;
}

async function errText(res: Response, action: string): Promise<string> {
  return extractErrorMessage(res, `Failed to ${action} (${res.status})`);
}

/**
 * Submits a population file set for an OE control via the same backend-proxied
 * flow as evidence (see useSubmitEvidence):
 *   1. GET  .../population/upload-link            -> folderPath for the active round
 *   2. per file: POST .../population/upload        -> multipart; backend proxies to Azure
 *   3. POST .../population/submit { folderPath }   -> backend records files + advances
 *                                                     the control to POPULATION_INTERNAL_REVIEW
 */
export function useSubmitPopulation() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId, files, attestation }: SubmitPopulationPayload): Promise<void> => {
      if (files.length === 0 && !attestation) throw new Error("Select at least one file, or add a note, to submit.");
      const base = `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/population`;

      // 1. Folder path for the active population round.
      const linkRes = await authFetch(`${base}/upload-link`);
      if (!linkRes.ok) throw new Error(await errText(linkRes, "start population upload"));
      const { folderPath } = (await linkRes.json()) as { folderPath: string };

      // 2. Proxy each file through the backend (multipart).
      for (const file of files) {
        const form = new FormData();
        form.append("folderPath", folderPath);
        form.append("file", file, file.name);

        const upRes = await authFetch(`${base}/upload`, { method: "POST", body: form });
        if (!upRes.ok) throw new Error(await errText(upRes, `upload ${file.name}`));
      }

      // 3. Record the submission — the backend lists the blobs and advances status.
      const submitRes = await authFetch(`${base}/submit`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ folderPath, attestation: attestation ?? undefined }),
      });
      if (!submitRes.ok) throw new Error(await errText(submitRes, "submit population"));
    },

    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: populationQueryKey(auditId, controlId) });
    },
  });
}

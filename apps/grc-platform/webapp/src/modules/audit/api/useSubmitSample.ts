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

interface SubmitSamplePayload {
  auditId: number;
  controlId: number;
  files: File[];
  note: string;
}

async function errText(res: Response, action: string): Promise<string> {
  return extractErrorMessage(res, `Failed to ${action} (${res.status})`);
}

/**
 * The auditor selects and submits the sample: files and/or a note (at least one
 * is required — neither is individually mandatory), via the same
 * backend-proxied flow as evidence/population (see useSubmitEvidence):
 *   1. GET  .../sample/upload-link             -> folderPath (a "sample/" subfolder
 *                                                  of the population round)
 *   2. per file: POST .../sample/upload         -> multipart; backend proxies to Azure
 *   3. POST .../sample/submit { folderPath, note } -> backend records files, sets the
 *                                                     sample note, advances to SUBMITTED_SAMPLE
 */
export function useSubmitSample() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId, files, note }: SubmitSamplePayload): Promise<void> => {
      if (files.length === 0 && note.trim() === "") {
        throw new Error("Provide sample files, a note, or both.");
      }
      const base = `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/sample`;

      const linkRes = await authFetch(`${base}/upload-link`);
      if (!linkRes.ok) throw new Error(await errText(linkRes, "start sample upload"));
      const { folderPath } = (await linkRes.json()) as { folderPath: string };

      for (const file of files) {
        const form = new FormData();
        form.append("folderPath", folderPath);
        form.append("file", file, file.name);

        const upRes = await authFetch(`${base}/upload`, { method: "POST", body: form });
        if (!upRes.ok) throw new Error(await errText(upRes, `upload ${file.name}`));
      }

      const submitRes = await authFetch(`${base}/submit`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ folderPath, note }),
      });
      if (!submitRes.ok) throw new Error(await errText(submitRes, "submit sample"));
    },

    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: populationQueryKey(auditId, controlId) });
    },
  });
}

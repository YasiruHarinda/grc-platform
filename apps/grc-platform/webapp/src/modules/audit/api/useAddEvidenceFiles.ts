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
import { evidenceQueryKey } from "@modules/audit/api/useGetEvidence";
import { extractErrorMessage } from "@modules/audit/api/apiError";

interface AddEvidenceFilesPayload {
  auditId: number;
  controlId: number;
  files: File[];
}

async function errText(res: Response, action: string): Promise<string> {
  return extractErrorMessage(res, `Failed to ${action} (${res.status})`);
}

/**
 * Adds files to the control's CURRENT evidence round (POST .../evidence/files)
 * instead of starting a new one via useSubmitEvidence — this is what "Add
 * Files" must use while a submission is still under internal review.
 *
 * Before this hook existed, Add Files called useSubmitEvidence directly, which
 * always creates a brand-new evidence round. Since a reviewer's later decision
 * only closes out the single latest round, any earlier round created by an
 * in-review Add Files click was left stranded in SUBMITTED status forever —
 * silently resurfacing its files in SubmittedEvidenceList alongside every
 * future resubmission.
 */
export function useAddEvidenceFiles() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId, files }: AddEvidenceFilesPayload): Promise<void> => {
      if (files.length === 0) throw new Error("Select at least one file to add.");
      const base = `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/evidence`;

      const linkRes = await authFetch(`${base}/upload-link`);
      if (!linkRes.ok) throw new Error(await errText(linkRes, "start evidence upload"));
      const { folderPath } = (await linkRes.json()) as { folderPath: string };

      const uploaded: { blobName: string; fileName: string }[] = [];
      for (const file of files) {
        const form = new FormData();
        form.append("folderPath", folderPath);
        form.append("file", file, file.name);

        const upRes = await authFetch(`${base}/upload`, { method: "POST", body: form });
        if (!upRes.ok) throw new Error(await errText(upRes, `upload ${file.name}`));
        const { blobName } = (await upRes.json()) as { blobName: string };
        uploaded.push({ blobName, fileName: file.name });
      }

      const addRes = await authFetch(`${base}/files`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ files: uploaded }),
      });
      if (!addRes.ok) throw new Error(await errText(addRes, "add evidence files"));
    },

    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: evidenceQueryKey(auditId, controlId) });
    },
  });
}

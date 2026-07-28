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
import { populationQueryKey } from "@modules/audit/api/useGetPopulation";
import { extractErrorMessage } from "@modules/audit/api/apiError";

interface DeletePopulationFilePayload {
  auditId: number;
  controlId: number;
  fileId: number;
}

/**
 * Removes a single population/sample file (see DELETE .../population/files/{fileId}).
 * The backend also deletes the underlying blob, so the file can't resurface the
 * next time its (stable, reused) upload folder is re-listed on submit.
 */
export function useDeletePopulationFile() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId, fileId }: DeletePopulationFilePayload): Promise<void> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/population/files/${fileId}`,
        { method: "DELETE" },
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to remove file (${res.status})`));
      }
    },
    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: populationQueryKey(auditId, controlId) });
    },
  });
}

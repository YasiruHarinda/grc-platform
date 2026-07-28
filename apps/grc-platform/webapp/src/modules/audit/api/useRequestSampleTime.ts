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
import { extractErrorMessage } from "@modules/audit/api/apiError";

interface RequestTimePayload {
  auditId: number;
  controlId: number;
}

/** Auditor signals they need more time before selecting a sample: POPULATION_COMPLETE → AWAITING_SAMPLE. */
export function useRequestSampleTime() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId }: RequestTimePayload): Promise<{ status: string }> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/sample/request-time`,
        { method: "POST" },
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to request more time (${res.status})`));
      }
      return res.json() as Promise<{ status: string }>;
    },
    onSuccess: (_data, { auditId }) => {
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
    },
  });
}

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
import { frameworkControlsQueryKey } from "@modules/audit/api/useGetFrameworkControls";
import type { AuditFrameworkControl, CreateFrameworkControlRequest } from "@modules/audit/types/audit";

export function useCreateFrameworkControl(frameworkId: number) {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (req: CreateFrameworkControlRequest): Promise<AuditFrameworkControl> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audit/frameworks/${frameworkId}/controls`,
        { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(req) },
      );
      if (!res.ok) {
        const body = await res.json().catch(() => null) as { message?: string } | null;
        throw new Error(body?.message || `Failed to create framework control (${res.status})`);
      }
      return res.json() as Promise<AuditFrameworkControl>;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: frameworkControlsQueryKey(frameworkId) });
    },
  });
}

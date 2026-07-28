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
import { AUDITS_QUERY_KEY } from "@modules/audit/api/useGetAudits";
import { AUDIT_DASHBOARD_QUERY_KEY } from "@modules/audit/api/useGetDashboard";
import type { AuditStatus } from "@modules/audit/types/audit";
import { extractErrorMessage } from "@modules/audit/api/apiError";

/** Changes an audit's lifecycle status (e.g. archive / unarchive). */
export function useUpdateAuditStatus() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, status }: { auditId: number; status: AuditStatus }) => {
      const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audits/${auditId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status }),
      });
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to update audit status (${res.status})`));
      }
    },
    onSuccess: () => {
      // AUDITS_QUERY_KEY (["audits"]) prefix-matches the single-audit key too.
      void queryClient.invalidateQueries({ queryKey: AUDITS_QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: AUDIT_DASHBOARD_QUERY_KEY });
    },
  });
}

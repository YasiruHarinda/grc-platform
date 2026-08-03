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
import { auditQueryKey } from "@modules/audit/api/useGetAudit";
import { evidenceQueryKey } from "@modules/audit/api/useGetEvidence";
import { extractErrorMessage } from "@modules/audit/api/apiError";

interface DecisionPayload {
  auditId: number;
  controlId: number;
  decision: "APPROVE" | "REJECT";
  comment?: string;
}

/**
 * Assigned auditor's decision on evidence that passed internal review (see
 * evidence/validate). This is the auditor-gated counterpart to the internal
 * reviewer's decision at EVIDENCE_INTERNAL_REVIEW (see useReviewEvidence).
 */
export function useValidateEvidence() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId, decision, comment }: DecisionPayload): Promise<{ status: string }> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/evidence/validate`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ decision, comment: comment ?? null }),
        },
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to record decision (${res.status})`));
      }
      return res.json() as Promise<{ status: string }>;
    },
    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: auditQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: evidenceQueryKey(auditId, controlId) });
    },
  });
}

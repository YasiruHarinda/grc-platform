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
import { populationQueryKey } from "@modules/audit/api/useGetPopulation";
import type { ControlStatus } from "@modules/audit/types/audit";

interface OverrideStatusPayload {
  auditId: number;
  controlId: number;
  status: ControlStatus;
}

// useOverrideControlStatus drives the admin backward status override
// (ManageControls-gated) — distinct from the ordinary forward workflow's
// status PATCH, and hitting its own endpoint (/status/override).
export function useOverrideControlStatus() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ auditId, controlId, status }: OverrideStatusPayload) => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/status/override`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ status }),
        },
      );
      if (!res.ok) throw new Error(`Failed to override control status (${res.status})`);
    },

    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: auditQueryKey(auditId) });
      // The override cascades the control's dependent evidence/population round
      // too (see OverrideControlStatus's demoteEvidenceRound/demotePopulationRound
      // in the entity) — without invalidating these, SubmittedEvidenceList /
      // SubmittedPopulationFiles keep showing their stale cached round list
      // until some unrelated action (e.g. a new upload) happens to invalidate
      // it, same bug useReviewEvidence/useValidateEvidence already avoid by
      // invalidating evidenceQueryKey on their own reject path.
      void queryClient.invalidateQueries({ queryKey: evidenceQueryKey(auditId, controlId) });
      void queryClient.invalidateQueries({ queryKey: populationQueryKey(auditId, controlId) });
    },
  });
}

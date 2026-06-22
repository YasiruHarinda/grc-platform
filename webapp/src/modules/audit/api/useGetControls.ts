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
import { getMockControls } from "@modules/audit/mock/auditMock";
import type { ControlListResponse } from "@modules/audit/types/audit";

export const controlsQueryKey = (auditId: number) =>
  ["audits", auditId, "controls"] as const;

export function useGetControls(auditId: number) {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: controlsQueryKey(auditId),
    queryFn: async (): Promise<ControlListResponse> => {
      if (!BACKEND_BASE_URL || window.config?.GRC_PLATFORM_MOCK_AUTH === true) {
        return getMockControls(auditId);
      }
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls`,
      );
      if (!res.ok) throw new Error(`Failed to load controls (${res.status})`);
      return res.json() as Promise<ControlListResponse>;
    },
    enabled: auditId > 0,
  });
}

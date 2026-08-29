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
import type { DashboardData } from "@modules/audit/types/dashboard";

export const AUDIT_DASHBOARD_QUERY_KEY = ["audit", "dashboard"] as const;

export function useGetDashboard() {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: AUDIT_DASHBOARD_QUERY_KEY,
    queryFn: async (): Promise<DashboardData> => {
      const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audits/dashboard`);
      if (!res.ok) throw new Error(`Failed to load dashboard (${res.status})`);
      return res.json() as Promise<DashboardData>;
    },
    staleTime: 2 * 60 * 1000,
  });
}

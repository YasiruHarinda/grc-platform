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
import type { AuditUser } from "@modules/audit/types/user";

// TODO: replace with real mock data when available
const MOCK_USERS: AuditUser[] = [];

/**
 * Fetches the list of WSO2 users available for control assignment.
 *
 * Backend responsibility: call Asgardeo SCIM2 GET /scim2/Users using a
 * service-account token, map to AuditUser, and return the list.
 * Endpoint: GET /api/v1/users
 */
export function useGetUsers() {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: ["users"] as const,
    queryFn: async (): Promise<AuditUser[]> => {
      if (!BACKEND_BASE_URL || window.config?.GRC_PLATFORM_MOCK_AUTH === true) {
        return MOCK_USERS;
      }
      const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/users`);
      if (!res.ok) throw new Error(`Failed to load users (${res.status})`);
      return res.json() as Promise<AuditUser[]>;
    },
    staleTime: 5 * 60 * 1000, // user list changes rarely — cache for 5 min
  });
}

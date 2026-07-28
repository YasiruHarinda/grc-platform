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

import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import { extractErrorMessage } from "@modules/audit/api/apiError";
import type { TrailEntry } from "@modules/audit/api/useGetTrail";

/** Page size for the audit-wide Activity Log. Mirrors the backend's
 * defaultAuditTrailLimit — kept larger than a typical list page since rows are
 * rendered compact, so fewer "next page" clicks are needed to scan a long log. */
export const activityLogPageSize = 50;

export interface AuditActivityFilters {
  /** Multiple values are OR'd, matching the multi-select Control filter. */
  controlIds?: number[];
  /** Inclusive, YYYY-MM-DD. */
  from?: string;
  /** Inclusive, YYYY-MM-DD. */
  to?: string;
}

interface AuditActivityListResponse {
  items: TrailEntry[];
  total: number;
}

export const auditActivityQueryKey = (
  auditId: number,
  filters: AuditActivityFilters,
  page: number,
) => ["audit", "activity", auditId, filters, page] as const;

/** Fetches the whole audit's activity (audit-level and every control's events
 * together), newest first, narrowed by filters and paginated. `page` is
 * 0-indexed, matching MUI's TablePagination. */
export function useGetAuditActivity(auditId: number, filters: AuditActivityFilters, page: number) {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: auditActivityQueryKey(auditId, filters, page),
    enabled: auditId > 0,
    placeholderData: keepPreviousData,
    queryFn: async (): Promise<AuditActivityListResponse> => {
      const params = new URLSearchParams();
      params.set("limit", String(activityLogPageSize));
      params.set("offset", String(page * activityLogPageSize));
      for (const id of filters.controlIds ?? []) params.append("controlId", String(id));
      if (filters.from) params.set("from", filters.from);
      if (filters.to) params.set("to", filters.to);

      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/trail?${params.toString()}`,
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to load activity log (${res.status})`));
      }
      const body = (await res.json()) as AuditActivityListResponse;
      return { items: body.items ?? [], total: body.total ?? 0 };
    },
  });
}

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
import type { ActionItem } from "@modules/audit/types/dashboard";

export type WorkQueueTab = "action-items" | "due-soon" | "pending" | "validation" | "overdue";

export interface WorkQueuePage {
  items: ActionItem[];
  total: number;
  page: number;
  limit: number;
}

const LIMIT = 25;

export type DueSort = "asc" | "desc";

export interface WorkQueueFilters {
  teamIds?: number[];
  ownerIds?: number[];
  auditIds?: number[];
  statuses?: string[];
  controlNumber?: string;
  dueSort?: DueSort;
}

export function useGetWorkQueue(tab: WorkQueueTab, page: number, filters: WorkQueueFilters = {}, enabled = true) {
  const authFetch = useAuthApiClient();
  const {
    teamIds = [], ownerIds = [], auditIds = [], statuses = [],
    controlNumber = "", dueSort = "asc",
  } = filters;

  return useQuery({
    queryKey: ["audit", "work-queue", tab, page, teamIds, ownerIds, auditIds, statuses, controlNumber, dueSort] as const,
    queryFn: async (): Promise<WorkQueuePage> => {
      const params = new URLSearchParams({ tab, page: String(page), limit: String(LIMIT) });
      teamIds.forEach((id) => params.append("teamIds", String(id)));
      ownerIds.forEach((id) => params.append("ownerIds", String(id)));
      auditIds.forEach((id) => params.append("auditIds", String(id)));
      statuses.forEach((s) => params.append("statuses", s));
      if (controlNumber.trim()) params.set("controlNumber", controlNumber.trim());
      if (dueSort === "desc") params.set("dueSort", "desc");
      const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audits/work-queue?${params.toString()}`);
      if (!res.ok) throw new Error(`Failed to load work queue (${res.status})`);
      return res.json() as Promise<WorkQueuePage>;
    },
    staleTime: 60 * 1000,
    enabled,
  });
}

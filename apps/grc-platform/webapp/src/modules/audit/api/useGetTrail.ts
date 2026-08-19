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
import { extractErrorMessage } from "@modules/audit/api/apiError";

/** One immutable event in a control's history (append-only audit trail). */
export interface TrailEntry {
  id: number;
  action:
    | "CREATED"
    | "UPLOADED"
    | "RESUBMITTED"
    | "APPROVED"
    | "REJECTED"
    | "COMMENTED"
    | "ESCALATED"
    | "AI_VALIDATED"
    | "EXPORTED"
    | "UPDATED"
    | "DELETED"
    | "OVERRIDDEN";
  controlId: number | null;
  evidenceId: number | null;
  createdBy: string;
  createdAt: string;
  // Free-form JSON recorded with the event (e.g. { from, to, via, comment,
  // controlNumber }). Passed through verbatim by the backend; may be absent.
  details?: TrailDetails | null;
}

/** The union of detail keys the platform records; all optional. */
export interface TrailDetails {
  from?: string;
  to?: string;
  via?: string;
  comment?: string;
  isInternal?: boolean;
  // File names submitted with an UPLOADED/RESUBMITTED evidence action.
  files?: string[];
  controlNumber?: string;
  // Audit-level CREATED/DELETED carry name; UPDATED carries whichever fields
  // changed (periodStart/periodEnd/scopeDescription/status).
  name?: string;
  periodStart?: string;
  periodEnd?: string;
  scopeDescription?: string;
  status?: string;
  [key: string]: unknown;
}

interface TrailListResponse {
  items: TrailEntry[];
  total: number;
}

export const trailQueryKey = (auditId: number, controlId: number) =>
  ["audit", "trail", auditId, controlId] as const;

/** Fetches a control's history (newest first from the backend). */
export function useGetTrail(auditId: number, controlId: number, enabled: boolean) {
  const authFetch = useAuthApiClient();

  return useQuery({
    queryKey: trailQueryKey(auditId, controlId),
    enabled,
    queryFn: async (): Promise<TrailEntry[]> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/trail`,
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to load history (${res.status})`));
      }
      const body = (await res.json()) as TrailListResponse;
      return body.items ?? [];
    },
  });
}

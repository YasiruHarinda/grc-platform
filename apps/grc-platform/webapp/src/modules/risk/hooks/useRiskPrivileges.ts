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

import { useCallback, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";

const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

// Shared across all hook instances so only one fetch ever fires per session.
let _promise: Promise<Set<string> | null> | null = null;

export interface RiskPrivilegeState {
  can: (privilege: string) => boolean;
  loading: boolean;
}

// Fetches the current user's resolved privilege list from GET /api/v1/me/privileges
// once per session. All hook instances (SideBar, PrivilegeGuard, page components)
// share the same promise — no duplicate requests.
// In mock-auth mode all privileges are granted immediately without an API call.
//
// ⚠ THIS IS THE UNION ACROSS EVERY REGISTER THE CALLER HAS A GRANT IN.
//
// Use it for NAV AND ROUTE GATING ONLY — "should this tab exist at all". It is
// the wrong answer for any decision about a specific risk: a user who is Risk
// Owner in one register and read-only in another holds RISK_OWNER_APPROVE here,
// so gating an Approve button on it would show that button on risks the server
// then refuses with a 403.
//
// For per-risk decisions use the risk's own `effective_privileges`, which the
// server resolves in that risk's source register — see RiskDetailDrawer.
//
// Roles now live in this platform's database (user_role_grant), not in
// Asgardeo; this endpoint's response shape is unchanged by that move.
export function useRiskPrivileges(): RiskPrivilegeState {
  const authFetch = useAuthApiClient();
  const [privileges, setPrivileges] = useState<Set<string> | null>(new Set());
  const [loading, setLoading] = useState(!isMockAuth);

  useEffect(() => {
    if (isMockAuth) return;
    if (!_promise) {
      _promise = authFetch(`${BACKEND_BASE_URL}/api/v1/me/privileges`)
        .then((res) => res.json() as Promise<{ privileges?: string[]; allowAll?: boolean }>)
        .then((data) => data.allowAll ? null : new Set<string>(data.privileges ?? []))
        .catch(() => { _promise = null; return new Set<string>(); });
    }
    let cancelled = false;
    _promise.then((privs) => {
      if (!cancelled) {
        setPrivileges(privs);
        setLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // intentionally empty — _promise deduplicates across instances and renders

  const can = useCallback(
    (priv: string) => isMockAuth || privileges === null || privileges.has(priv),
    [privileges],
  );

  return { can, loading };
}

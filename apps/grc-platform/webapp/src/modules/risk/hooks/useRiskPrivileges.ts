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

// Shared across every hook instance mounted at once, so a burst of
// simultaneous mounts (SideBar + every PrivilegeGuard on a page) still fires
// only one request — but cleared as soon as it resolves, not kept for the
// rest of the page's lifetime, so a later mount (a new route, or a
// logout/login within the same SPA session without a full reload) fetches
// fresh rather than reusing a stale/wrong-subject privilege set.
let _promise: Promise<Set<string> | null> | null = null;

export interface RiskPrivilegeState {
  can: (privilege: string) => boolean;
  loading: boolean;
}

// Fetches the current user's resolved privilege list from GET /api/v1/me/privileges.
// All hook instances mounted at the same time (SideBar, PrivilegeGuard, page
// components) share one in-flight request — no duplicate requests for a
// simultaneous mount burst — but each new mount after that fetches fresh; see
// _promise's own comment for why.
//
// Always calls the real endpoint, mock-auth mode included. This used to
// short-circuit to "every privilege granted" whenever GRC_PLATFORM_MOCK_AUTH
// was true — correct back when mock auth meant no backend identity existed to
// check at all, wrong now that mock auth carries a forged token whose sub the
// backend resolves real grants for (see the GRC_PLATFORM_MOCK_AUTH_TOKEN
// wiring in useAuthApiClient). With the old shortcut, revoking every grant
// locally still left every nav tab and route visible — nothing to test
// against, since the frontend never even asked. The endpoint itself still
// reports `allowAll: true` in true local-dev (no privilege store configured
// on the backend), so this stays correct in that mode too — nothing here
// assumes the backend is in any particular mode.
export function useRiskPrivileges(): RiskPrivilegeState {
  const authFetch = useAuthApiClient();
  const [privileges, setPrivileges] = useState<Set<string> | null>(new Set());
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!_promise) {
      _promise = authFetch(`${BACKEND_BASE_URL}/api/v1/me/privileges`)
        .then((res) => res.json() as Promise<{ privileges?: string[]; allowAll?: boolean }>)
        .then((data) => {
          _promise = null;
          return data.allowAll ? null : new Set<string>(data.privileges ?? []);
        })
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
    (priv: string) => privileges === null || privileges.has(priv),
    [privileges],
  );

  return { can, loading };
}

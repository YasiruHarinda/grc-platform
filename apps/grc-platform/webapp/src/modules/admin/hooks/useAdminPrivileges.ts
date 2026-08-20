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

// Deliberately a near-duplicate of modules/risk/hooks/useRiskPrivileges.ts
// rather than importing it: that hook is named/owned by the Risk module, and
// reaching across module boundaries for a generic "resolved privilege set"
// concept is the wrong direction to couple two modules that are supposed to
// stay independent (see SideBar.tsx's own comment on module ownership). The
// cost is one extra GET /me/privileges call per page load (each hook's
// in-flight-request cache is its own module-level variable, so the two don't
// dedupe against each other) — cheap and worth it to keep modules/admin from
// depending on modules/risk. A future cleanup could extract both to a shared
// location if a third module ever needs the same thing.
let _promise: Promise<Set<string> | null> | null = null;

export interface AdminPrivilegeState {
  can: (privilege: string) => boolean;
  loading: boolean;
}

// Fetches the current user's resolved privilege list from GET /api/v1/me/privileges.
export function useAdminPrivileges(): AdminPrivilegeState {
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

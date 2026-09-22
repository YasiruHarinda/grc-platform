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

// Absolute URLs for the Audit Hub. routes.tsx declares the same tree as
// relative segments under <Route path="audit">, mounted at the app root;
// navigate() needs the full path, so it lives here, once. Moving the module
// means editing AUDIT_BASE (and the mount in App.tsx), not grepping for
// literals. Kept out of routes.tsx so pages can import it without a
// routes -> pages -> routes cycle.
const AUDIT_BASE = "/audit";

type Id = number | string;

function withQuery(path: string, query: Record<string, Id | null | undefined>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== null && value !== undefined) params.set(key, String(value));
  }
  const qs = params.toString();
  return qs ? `${path}?${qs}` : path;
}

export const auditPaths = {
  /** Audits list, optionally pre-filtered to one framework. */
  list: (frameworkId?: Id | null) => withQuery(`${AUDIT_BASE}/audits`, { framework: frameworkId }),
  /** Create-audit wizard, optionally pre-selecting a framework. */
  create: (frameworkId?: Id | null) => withQuery(`${AUDIT_BASE}/audits/create`, { framework: frameworkId }),
  /** Audit detail, optionally deep-linked to one control. */
  detail: (auditId: Id, controlId?: Id | null) =>
    withQuery(`${AUDIT_BASE}/audits/${auditId}`, { control: controlId }),
  activity: (auditId: Id) => `${AUDIT_BASE}/audits/${auditId}/activity`,
  dashboard: `${AUDIT_BASE}/dashboard`,
};

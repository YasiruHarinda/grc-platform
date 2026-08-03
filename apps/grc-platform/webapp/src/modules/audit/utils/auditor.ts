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

import type { AuditControl } from "@modules/audit/types/audit";

const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

/**
 * True when the signed-in user is this control's assigned auditor POC — the
 * same check the backend enforces (matching control.auditorEmail against the
 * caller's token email; see requireAssignedAuditor). This is UI gating only: a
 * mismatch here just hides a card/button, it never grants access, since the
 * backend re-derives and enforces the check independently.
 *
 * In mock-auth mode (no real IdP) every auditor-only surface is shown, mirroring
 * how useAuditPrivileges.can() allows everything in that mode.
 */
export function isAssignedAuditor(control: Pick<AuditControl, "auditorEmail">, currentUserEmail: string | null): boolean {
  if (isMockAuth) return true;
  if (!currentUserEmail || !control.auditorEmail) return false;
  return control.auditorEmail.toLowerCase() === currentUserEmail.toLowerCase();
}

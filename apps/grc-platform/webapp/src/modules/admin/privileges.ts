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

// Admin Console privilege name constants.
// Values must match privilege_name in the privilege table and the constants
// in backend/internal/shared/privilege/privilege.go exactly.
//
// Not re-exported from modules/audit/privileges — despite what
// ADMIN_CONSOLE_DESIGN.md's handover notes assumed, MANAGE_USERS was not
// actually defined anywhere in the webapp yet (grepped before writing this;
// it exists only in the Go privilege package). Declared fresh here instead.
export const AdminPrivilege = {
  ManageUsers: "MANAGE_USERS",
  // Not used by anything built in this phase (Risk Teams/Categories/
  // Compliance References/Risk Scores are a later phase; the Manage Audit
  // Hub tab is a stub with no gated content of its own yet) — declared now
  // so the later phases don't have to touch this file again.
  ManageRiskHub: "MANAGE_RISK_HUB",
  ManageAuditHub: "MANAGE_AUDIT_HUB",
} as const;

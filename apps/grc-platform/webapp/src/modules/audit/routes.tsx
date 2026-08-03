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

import { Route, Navigate } from "react-router";
import AuditDashboard from "@modules/audit/pages/AuditDashboard";
import AuditsListPage from "@modules/audit/pages/AuditsListPage";
import AuditDetailPage from "@modules/audit/pages/AuditDetailPage";
import AuditActivityLogPage from "@modules/audit/pages/AuditActivityLogPage";
import CreateAuditPage from "@modules/audit/pages/CreateAuditPage";
import AuditPrivilegeGuard from "@modules/audit/components/AuditPrivilegeGuard";
import { AuditPrivilege } from "@modules/audit/privileges";

// Audit Hub routes, mounted under /audit by App.tsx. Owned by the Audit module —
// add Audit pages here without touching the shared App.tsx.
export const auditRoutes = (
  <Route path="audit">
    <Route index element={<Navigate to="audits" replace />} />
    <Route path="audits" element={<AuditsListPage />} />
    <Route path="audits/create" element={<AuditPrivilegeGuard privilege={AuditPrivilege.CreateAudit}><CreateAuditPage /></AuditPrivilegeGuard>} />
    <Route path="audits/:auditId" element={<AuditDetailPage />} />
    <Route path="audits/:auditId/activity" element={<AuditActivityLogPage />} />
    <Route path="dashboard" element={<AuditDashboard />} />
  </Route>
);

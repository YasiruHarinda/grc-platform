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

import { Navigate, Route } from "react-router";
import ManageAuditHubPage from "@modules/admin/pages/ManageAuditHubPage";
import ManageRiskHubPage from "@modules/admin/pages/ManageRiskHubPage";
import UsersPage from "@modules/admin/pages/UsersPage";
import PrivilegeGuard from "./components/PrivilegeGuard";
import { AdminPrivilege } from "./privileges";

// Admin Console routes, mounted under /admin by App.tsx. Owned by the Admin
// module — add Admin pages here without touching the shared App.tsx.
export const adminRoutes = (
  <Route path="admin">
    <Route index element={<Navigate to="users" replace />} />
    <Route
      path="users"
      element={
        <PrivilegeGuard privilege={AdminPrivilege.ManageUsers}>
          <UsersPage />
        </PrivilegeGuard>
      }
    />
    <Route
      path="risk-hub"
      element={
        <PrivilegeGuard privilege={AdminPrivilege.ManageRiskHub}>
          <ManageRiskHubPage />
        </PrivilegeGuard>
      }
    />
    <Route
      path="audit-hub"
      element={
        <PrivilegeGuard privilege={AdminPrivilege.ManageAuditHub}>
          <ManageAuditHubPage />
        </PrivilegeGuard>
      }
    />
  </Route>
);

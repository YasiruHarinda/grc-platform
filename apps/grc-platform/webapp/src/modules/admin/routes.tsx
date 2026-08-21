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

import { Box, CircularProgress } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { Navigate, Route } from "react-router";
import Error403Page from "@components/error/Error403Page";
import ManageAuditHubPage from "@modules/admin/pages/ManageAuditHubPage";
import ManageRiskHubPage from "@modules/admin/pages/ManageRiskHubPage";
import UsersPage from "@modules/admin/pages/UsersPage";
import PrivilegeGuard from "./components/PrivilegeGuard";
import { useAdminPrivileges } from "./hooks/useAdminPrivileges";
import { AdminPrivilege } from "./privileges";

// Redirects /admin to the first sub-page the caller actually holds a
// privilege for, in the same priority order as adminNav's item list — rather
// than always targeting /admin/users, which 403'd anyone whose only Admin
// privilege was ManageRiskHub or ManageAuditHub.
function AdminIndexRedirect(): JSX.Element {
  const { can, loading } = useAdminPrivileges();

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", height: 200 }}>
        <CircularProgress />
      </Box>
    );
  }
  if (can(AdminPrivilege.ManageUsers)) return <Navigate to="users" replace />;
  if (can(AdminPrivilege.ManageRiskHub)) return <Navigate to="risk-hub" replace />;
  if (can(AdminPrivilege.ManageAuditHub)) return <Navigate to="audit-hub" replace />;
  return <Error403Page />;
}

// Admin Console routes, mounted under /admin by App.tsx. Owned by the Admin
// module — add Admin pages here without touching the shared App.tsx.
export const adminRoutes = (
  <Route path="admin">
    <Route index element={<AdminIndexRedirect />} />
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

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
import type { JSX, ReactNode } from "react";
import Error403Page from "@components/error/Error403Page";
import { useAdminPrivileges } from "../hooks/useAdminPrivileges";

interface PrivilegeGuardProps {
  privilege: string;
  children: ReactNode;
}

// Renders children only when the current user holds the required privilege.
// Shows a spinner while privileges are loading and Error403Page when denied.
//
// Belt-and-suspenders with the sidebar's hideSectionWithoutPrivilege: that
// hides the nav entry, this refuses the route itself if reached directly by
// URL (a bookmark, a shared link, back-button history).
export default function PrivilegeGuard({ privilege, children }: PrivilegeGuardProps): JSX.Element {
  const { can, loading } = useAdminPrivileges();

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", height: 200 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (!can(privilege)) {
    return <Error403Page />;
  }

  return <>{children}</>;
}

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

import { useAdminPrivileges } from "@modules/admin/hooks/useAdminPrivileges";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { useRiskPrivileges } from "@modules/risk/hooks/useRiskPrivileges";
import type { SectionPrivResolver } from "./resolveVisibleNav";

// Assembles the section id → privilege-resolver map that resolveVisibleNav
// consumes, by calling each module's useXPrivileges hook.
//
// Both consumers of resolveVisibleNav — SideBar and LandingRedirect — need this
// exact map, so it lives here rather than being rebuilt in each. Wiring a new
// module's section into the nav means adding its resolver here once, and adding
// its NavSection to sections.ts.
export function useSectionPrivileges(): Record<string, SectionPrivResolver> {
  const { can: canRisk, loading: riskLoading } = useRiskPrivileges();
  const { can: canAdmin, loading: adminLoading } = useAdminPrivileges();
  const { can: canAudit, loading: auditLoading } = useAuditPrivileges();

  return {
    risk: { can: canRisk, loading: riskLoading },
    admin: { can: canAdmin, loading: adminLoading },
    audit: { can: canAudit, loading: auditLoading },
  };
}

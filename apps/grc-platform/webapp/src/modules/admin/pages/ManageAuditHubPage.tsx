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

import { Box, Paper, Typography } from "@wso2/oxygen-ui";
import { Wrench } from "@wso2/oxygen-ui-icons-react";
import { type JSX } from "react";

// Stub only, per an explicit decision: Audit Hub reference-data management
// (teams/frameworks/products) is a later phase of this same project — the
// GRC backend doesn't even wire audit-team create/update yet, and framework/
// product creation today only exists inline inside CreateAuditPage, not as a
// standalone admin screen. This page exists so the console's nav shape and
// MANAGE_AUDIT_HUB gating are already correct when that phase lands.
export default function ManageAuditHubPage(): JSX.Element {
  return (
    <Box>
      <Typography variant="h4" fontWeight={700} sx={{ mb: 0.5 }}>
        Manage Audit Hub
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Reference data the Audit Hub's dropdowns are built from: audit teams, frameworks, and products.
      </Typography>
      <Paper variant="outlined" sx={{ p: 4, textAlign: "center" }}>
        <Wrench size={28} style={{ opacity: 0.5, marginBottom: 8 }} />
        <Typography variant="body1" fontWeight={600}>
          Coming in a later phase
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Audit Hub reference-data management isn't built yet. Today, frameworks and products are created inline from
          the Create Audit flow.
        </Typography>
      </Paper>
    </Box>
  );
}

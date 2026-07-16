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

import { Chip, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { AgingRiskItem } from "../../api/riskApi";
import { formatDate } from "../risk-registers/utils";

interface AgingRisksTableProps {
  data: AgingRiskItem[];
}

// Top open risks ranked by days since identification — the actionable list
// behind the "Avg. Days to Close" KPI tile: which specific risks are
// dragging the average up.
export default function AgingRisksTable({ data }: AgingRisksTableProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open risks.
      </Typography>
    );
  }

  return (
    <TableContainer>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Risk Code</TableCell>
            <TableCell sx={{ minWidth: 240 }}>Title</TableCell>
            <TableCell>Register</TableCell>
            <TableCell>Owner</TableCell>
            <TableCell>Level</TableCell>
            <TableCell>Identified</TableCell>
            <TableCell align="right">Age (days)</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {data.map((risk) => (
            <TableRow key={risk.id} hover>
              <TableCell>
                <Typography variant="body2" fontWeight={600} color="primary">
                  {risk.risk_code}
                </Typography>
              </TableCell>
              <TableCell sx={{ maxWidth: 260 }}>
                <Typography variant="body2" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {risk.risk_title}
                </Typography>
              </TableCell>
              <TableCell>{risk.register_name}</TableCell>
              <TableCell>{risk.owner_name || "—"}</TableCell>
              <TableCell>
                <Chip
                  label={risk.risk_level}
                  size="small"
                  sx={{ bgcolor: risk.color_code || undefined, color: risk.color_code ? "#fff" : undefined, fontWeight: 700 }}
                />
              </TableCell>
              <TableCell sx={{ whiteSpace: "nowrap" }}>{formatDate(risk.identified_date)}</TableCell>
              <TableCell align="right">
                <Typography variant="body2" fontWeight={600}>
                  {risk.age_days}
                </Typography>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

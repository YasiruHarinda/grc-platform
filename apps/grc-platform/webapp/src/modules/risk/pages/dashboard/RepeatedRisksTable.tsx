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

import {
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RepeatedComplianceRisk } from "../../api/riskApi";
import { CLOSED_COLOR, LEVEL_FALLBACK_COLORS, LEVEL_LABELS, OPEN_COLOR } from "./constants";

interface RepeatedRisksTableProps {
  data: RepeatedComplianceRisk[];
}

// Cert-tagged risks whose title recurs across two or more source registers.
// The description cell spans its per-register occurrence rows.
export default function RepeatedRisksTable({ data }: RepeatedRisksTableProps): JSX.Element {
  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No repeated risks impacting compliance certifications.
      </Typography>
    );
  }

  return (
    <TableContainer>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Risk Description</TableCell>
            <TableCell>Source</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>Residual Level</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {data.map((group) =>
            group.occurrences.map((occ, i) => (
              <TableRow key={`${group.risk_title}-${occ.register_name}-${i}`}>
                {i === 0 && (
                  <TableCell rowSpan={group.occurrences.length} sx={{ verticalAlign: "top" }}>
                    {group.risk_title}
                  </TableCell>
                )}
                <TableCell>{occ.register_name}</TableCell>
                <TableCell>
                  <Chip
                    size="small"
                    variant="outlined"
                    label={occ.status === "CLOSED" ? "Closed" : "Open"}
                    sx={{
                      borderColor: occ.status === "CLOSED" ? CLOSED_COLOR : OPEN_COLOR,
                      color: occ.status === "CLOSED" ? CLOSED_COLOR : OPEN_COLOR,
                    }}
                  />
                </TableCell>
                <TableCell>
                  <Chip
                    size="small"
                    label={LEVEL_LABELS[occ.risk_level] ?? occ.risk_level}
                    sx={{
                      bgcolor: `${occ.color_code || LEVEL_FALLBACK_COLORS[occ.risk_level]}26`,
                      border: `1px solid ${occ.color_code || LEVEL_FALLBACK_COLORS[occ.risk_level]}`,
                    }}
                  />
                </TableCell>
              </TableRow>
            )),
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

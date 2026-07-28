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

import { useEffect, useState } from "react";
import {
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { HighRiskItem } from "../../api/riskApi";
import { TREATMENT_COLORS, TREATMENT_LABELS } from "./constants";
import { formatDate } from "../risk-registers/utils";
import HighRiskDetailModal from "./HighRiskDetailModal";

interface HighRisksTableProps {
  data: HighRiskItem[];
}

const ROWS_PER_PAGE = 5;

// Org-wide open HIGH (residual) risks, oldest identified first, 5 per page.
// Clicking a row opens a read-only detail modal.
export default function HighRisksTable({ data }: HighRisksTableProps): JSX.Element {
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<HighRiskItem | null>(null);

  useEffect(() => {
    setPage(0);
  }, [data]);

  if (data.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No open high risks. 🎉
      </Typography>
    );
  }

  const pageRows = data.slice(page * ROWS_PER_PAGE, page * ROWS_PER_PAGE + ROWS_PER_PAGE);

  return (
    <TableContainer>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Source Register</TableCell>
            <TableCell sx={{ minWidth: 320 }}>Risk Title</TableCell>
            <TableCell>Risk Owner</TableCell>
            <TableCell>Identified Date</TableCell>
            <TableCell>Treatment</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {pageRows.map((risk) => {
            const treatmentColor = risk.treatment_strategy
              ? TREATMENT_COLORS[risk.treatment_strategy]
              : undefined;
            return (
              <TableRow
                key={risk.id}
                hover
                role="button"
                tabIndex={0}
                onClick={() => setSelected(risk)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setSelected(risk);
                  }
                }}
                sx={{ cursor: "pointer", verticalAlign: "top" }}
              >
                <TableCell sx={{ whiteSpace: "nowrap" }}>{risk.register_name}</TableCell>
                <TableCell>
                  <Typography variant="body2">{risk.risk_title}</Typography>
                </TableCell>
                <TableCell>{risk.owner_name || "—"}</TableCell>
                <TableCell sx={{ whiteSpace: "nowrap" }}>
                  {formatDate(risk.identified_date)}
                </TableCell>
                <TableCell sx={{ whiteSpace: "nowrap" }}>
                  {risk.treatment_strategy ? (
                    <Chip
                      size="small"
                      label={TREATMENT_LABELS[risk.treatment_strategy] ?? risk.treatment_strategy}
                      sx={{ bgcolor: `${treatmentColor}26`, border: `1px solid ${treatmentColor}` }}
                    />
                  ) : (
                    "—"
                  )}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
      <TablePagination
        component="div"
        count={data.length}
        page={page}
        onPageChange={(_, newPage) => setPage(newPage)}
        rowsPerPage={ROWS_PER_PAGE}
        rowsPerPageOptions={[ROWS_PER_PAGE]}
      />
      <HighRiskDetailModal risk={selected} onClose={() => setSelected(null)} />
    </TableContainer>
  );
}

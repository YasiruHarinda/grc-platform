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
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useNavigate } from "react-router";
import type { HighRiskItem } from "../../api/riskApi";
import { formatDate } from "../risk-registers/utils";
import { TREATMENT_COLORS, TREATMENT_LABELS } from "./constants";

interface HighRiskDetailModalProps {
  risk: HighRiskItem | null;
  onClose: () => void;
}

function Field({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="body2">{value}</Typography>
    </Box>
  );
}

// Read-only summary of one high-severity risk, reached by clicking a row in
// HighRisksTable. Editing/workflow actions live only on the Risk Registers
// page, which this links out to via a riskId deep link.
export default function HighRiskDetailModal({ risk, onClose }: HighRiskDetailModalProps): JSX.Element {
  const navigate = useNavigate();

  return (
    <Dialog open={risk !== null} onClose={onClose} maxWidth="sm" fullWidth>
      {risk && (
        <>
          <DialogTitle>{risk.risk_code}</DialogTitle>
          <Divider />
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <Field label="Source Register" value={risk.register_name} />
              <Field label="Risk Title" value={risk.risk_title} />
              <Field label="Risk Owner" value={risk.owner_name || "—"} />
              <Field label="Identified Date" value={formatDate(risk.identified_date)} />
              <Box>
                <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 0.5 }}>
                  Treatment
                </Typography>
                {risk.treatment_strategy ? (
                  <Chip
                    size="small"
                    label={TREATMENT_LABELS[risk.treatment_strategy] ?? risk.treatment_strategy}
                    sx={{
                      bgcolor: `${TREATMENT_COLORS[risk.treatment_strategy]}26`,
                      border: `1px solid ${TREATMENT_COLORS[risk.treatment_strategy]}`,
                    }}
                  />
                ) : (
                  <Typography variant="body2">—</Typography>
                )}
              </Box>
            </Stack>
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 2 }}>
            <Button onClick={onClose}>Close</Button>
            <Button
              variant="contained"
              onClick={() => navigate(`/risk/registers?riskId=${risk.id}`)}
            >
              Open in Risk Registers
            </Button>
          </DialogActions>
        </>
      )}
    </Dialog>
  );
}

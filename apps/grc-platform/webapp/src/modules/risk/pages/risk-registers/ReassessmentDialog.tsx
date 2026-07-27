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

import { useState } from "react";
import { toDateOnlyString } from "@utils/dateTime";
import {
  AdapterDateFns,
  Alert,
  Box,
  Button,
  DatePickers,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { CreateAssessmentPayload, RiskScore } from "../../api/riskApi";
import { dialogPaperSx } from "../cardStyles";
import RiskScoreGrid from "./RiskScoreGrid";

const { DatePicker, LocalizationProvider } = DatePickers;

interface ReassessmentDialogProps {
  open: boolean;
  riskCode: string;
  riskScores: RiskScore[];
  onClose: () => void;
  onSubmit: (payload: CreateAssessmentPayload) => Promise<void>;
}

export default function ReassessmentDialog({
  open,
  riskCode,
  riskScores,
  onClose,
  onSubmit,
}: ReassessmentDialogProps): JSX.Element {
  const [likelihood, setLikelihood] = useState(0);
  const [impact, setImpact] = useState(0);
  const [progress, setProgress] = useState("");
  const [reassessmentDate, setReassessmentDate] = useState<Date | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [apiError, setApiError] = useState("");

  const handleClose = () => {
    if (submitting) return;
    setLikelihood(0);
    setImpact(0);
    setProgress("");
    setReassessmentDate(null);
    setErrors({});
    setApiError("");
    onClose();
  };

  const validate = (): boolean => {
    const e: Record<string, string> = {};
    if (!likelihood || !impact) e.grid = "Please select a residual risk score from the grid.";
    if (!progress.trim()) e.progress = "Progress is required.";
    if (!reassessmentDate) {
      e.reassessmentDate = "Reassessment date is required.";
    } else {
      const today = new Date();
      today.setHours(0, 0, 0, 0);
      if (reassessmentDate < today) e.reassessmentDate = "Reassessment date cannot be in the past.";
    }
    setErrors(e);
    return Object.keys(e).length === 0;
  };

  const handleSubmit = async () => {
    if (!validate()) return;
    setSubmitting(true);
    setApiError("");
    try {
      await onSubmit({
        likelihood,
        impact,
        progress: progress.trim(),
        reassessment_date: toDateOnlyString(reassessmentDate)!,
      });
      handleClose();
    } catch (e: unknown) {
      setApiError(e instanceof Error ? e.message : "An error occurred.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{ sx: dialogPaperSx }}
    >
      <DialogTitle>
        <Typography variant="h6" fontWeight={700}>
          Reassess Risk
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {riskCode} : Record a residual risk assessment
        </Typography>
      </DialogTitle>
      <DialogContent>
        <Stack gap={3} sx={{ pt: 1 }}>
          {apiError && <Alert severity="error">{apiError}</Alert>}

          <Box>
            <Typography variant="body2" fontWeight={600} sx={{ mb: 1.5 }}>
              Residual Risk Score <span style={{ color: "red" }}>*</span>
            </Typography>
            <RiskScoreGrid
              riskScores={riskScores}
              likelihood={likelihood}
              impact={impact}
              onChange={(l, i) => {
                setLikelihood(l);
                setImpact(i);
                if (errors.grid) setErrors((prev) => ({ ...prev, grid: "" }));
              }}
              error={errors.grid}
            />
          </Box>

          <Divider />

          <TextField
            label="Progress"
            multiline
            rows={3}
            fullWidth
            required
            value={progress}
            onChange={(e) => {
              setProgress(e.target.value);
              if (errors.progress) setErrors((prev) => ({ ...prev, progress: "" }));
            }}
            error={!!errors.progress}
            helperText={errors.progress}
            placeholder="Describe the current remediation progress..."
            disabled={submitting}
          />

          <LocalizationProvider dateAdapter={AdapterDateFns}>
            <DatePicker
              label="Reassessment Date *"
              value={reassessmentDate}
              onChange={(d) => {
                setReassessmentDate(d);
                if (errors.reassessmentDate) setErrors((prev) => ({ ...prev, reassessmentDate: "" }));
              }}
              disablePast
              slotProps={{
                desktopPaper: {
                  sx: {
                    backdropFilter: "none",
                    backgroundColor: "#fff",
                    "[data-color-scheme='dark'] &": {
                      backgroundColor: "#1e1e1e",
                    },
                  },
                },
                textField: {
                  fullWidth: true,
                  error: !!errors.reassessmentDate,
                  helperText: errors.reassessmentDate,
                  disabled: submitting,
                },
              }}
            />
          </LocalizationProvider>
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={handleClose} disabled={submitting} color="inherit">
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={submitting}
          variant="contained"
        >
          {submitting ? "Saving..." : "Save Assessment"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

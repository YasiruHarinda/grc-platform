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

import { Controller, useFormContext, useWatch } from "react-hook-form";
import {
  AdapterDateFns,
  Box,
  Chip,
  DatePickers,
  Divider,
  FormHelperText,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Info } from "@wso2/oxygen-ui-icons-react";
import type { JSX, ReactNode } from "react";
import type { AddRiskFormValues, ImpactLevel, LikelihoodLevel } from "./types";
import type { RiskScore } from "../../api/riskApi";

const { DatePicker, LocalizationProvider } = DatePickers;

function FieldLabel({ children }: { children: ReactNode }): JSX.Element {
  return (
    <Typography
      variant="body2"
      fontWeight={500}
      color="text.primary"
      sx={{ display: "block", mb: 1 }}
    >
      {children}
    </Typography>
  );
}

function SectionHeader({ title }: { title: string }): JSX.Element {
  return (
    <Box>
      <Typography variant="subtitle1" fontWeight={600} color="text.primary">
        {title}
      </Typography>
      <Divider sx={{ mt: 1 }} />
    </Box>
  );
}

// Matrix: Likelihood Y-axis top (High 3) → bottom (Low 1), Impact X-axis left (Minor 1) → right (Major 3)
const LIKELIHOOD_ROWS: { value: LikelihoodLevel; label: string }[] = [
  { value: 3, label: "High 3" },
  { value: 2, label: "Medium 2" },
  { value: 1, label: "Low 1" },
];

const IMPACT_COLS: { value: ImpactLevel; label: string }[] = [
  { value: 1, label: "Minor 1" },
  { value: 2, label: "Moderate 2" },
  { value: 3, label: "Major 3" },
];

interface RiskAssessmentStepProps {
  riskScores: RiskScore[];
}

export default function RiskAssessmentStep({ riskScores }: RiskAssessmentStepProps): JSX.Element {
  const { control, setValue, clearErrors } = useFormContext<AddRiskFormValues>();

  const likelihood = useWatch({ control, name: "likelihood" });
  const impact     = useWatch({ control, name: "impact" });

  const findScore = (l: LikelihoodLevel, i: ImpactLevel): RiskScore | undefined =>
    riskScores.find(s => s.likelihood === l && s.impact === i);

  const selectedScore =
    likelihood != null && impact != null ? findScore(likelihood, impact) : null;

  const handleCellClick = (l: LikelihoodLevel, i: ImpactLevel): void => {
    setValue("likelihood", l, { shouldValidate: true, shouldDirty: true });
    setValue("impact",     i, { shouldValidate: true, shouldDirty: true });
    clearErrors("likelihood");
  };

  return (
    <LocalizationProvider dateAdapter={AdapterDateFns}>
      <Stack gap={4}>

        {/* ── Assessment Guide ─────────────────────────────────────────────── */}
        <Stack gap={3}>
          <SectionHeader title="Assessment Guide" />

          <Paper
            variant="outlined"
            sx={{ p: 2.5, borderRadius: 2, borderColor: "info.light" }}
          >
            <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 2.5 }}>
              <Info size={16} />
              <Typography variant="body2" fontWeight={600}>
                How to rate likelihood and impact
              </Typography>
            </Stack>

            <Box
              sx={{
                display: "flex",
                flexDirection: { xs: "column", sm: "row" },
                gap: 30,
                alignItems: "flex-start",
              }}
            >
              {/* Likelihood guide */}
              <Stack gap={1.5}>
                <Stack direction="row" alignItems="center" gap={0.75}>
                  <Typography variant="body2" fontWeight={600}>
                    Likelihood:
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    how often could this occur?
                  </Typography>
                </Stack>

                {[
                  { badge: "1 - Low",    desc: "The event could occur annually" },
                  { badge: "2 - Medium", desc: "The event may occur quarterly" },
                  { badge: "3 - High",   desc: "The event is expected to occur monthly" },
                ].map(({ badge, desc }) => (
                  <Stack key={badge} direction="row" gap={1.5} alignItems="flex-start">
                    <Box
                      sx={{
                        minWidth: 80,
                        px: 1,
                        py: 0.25,
                        borderRadius: 1,
                        textAlign: "center",
                        bgcolor: "action.hover",
                        border: "1px solid",
                        borderColor: "divider",
                        flexShrink: 0,
                      }}
                    >
                      <Typography variant="caption" fontWeight={700}>
                        {badge}
                      </Typography>
                    </Box>
                    <Typography variant="caption" color="text.secondary" sx={{ pt: 0.3 }}>
                      {desc}
                    </Typography>
                  </Stack>
                ))}
              </Stack>

              {/* Impact guide */}
              <Stack gap={1.5}>
                <Stack direction="row" alignItems="center" gap={0.75}>
                  <Typography variant="body2" fontWeight={600}>
                    Impact:
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    how severe are the consequences?
                  </Typography>
                </Stack>

                {[
                  {
                    badge: "1 - Minor",
                    line1: "Materializing the risk will not impact on any or some systems.",
                    line2: "There will be no instances where WSO2 critical information will be disclosed to an outside party.",
                  },
                  {
                    badge: "2 - Moderate",
                    line1: "Materializing the risk will impact all business systems excluding WSO2 offerings/Products.",
                    line2: "WSO2 internal information leakage.",
                  },
                  {
                    badge: "3 - Major",
                    line1: "Materializing the risk will create serious financial and reputational impact.",
                    line2: "Customer information, WSO2 critical information and sensitive information leakage.",
                  },
                ].map(({ badge, line1, line2 }) => (
                  <Stack key={badge} direction="row" gap={1.5} alignItems="flex-start">
                    <Box
                      sx={{
                        minWidth: 90,
                        px: 1,
                        py: 0.25,
                        borderRadius: 1,
                        textAlign: "center",
                        bgcolor: "action.hover",
                        border: "1px solid",
                        borderColor: "divider",
                        flexShrink: 0,
                      }}
                    >
                      <Typography variant="caption" fontWeight={700}>
                        {badge}
                      </Typography>
                    </Box>
                    <Stack gap={0.25} sx={{ pt: 0.3 }}>
                      <Typography variant="caption" color="text.secondary">{line1}</Typography>
                      <Typography variant="caption" color="text.secondary">{line2}</Typography>
                    </Stack>
                  </Stack>
                ))}
              </Stack>
            </Box>
          </Paper>
        </Stack>

        {/* ── Gross Risk Score Matrix ───────────────────────────────────────── */}
        <Stack gap={3}>
          <SectionHeader title="Gross Risk Score" />

          <Typography variant="body2" color="text.secondary">
            Click the cell that best represents the likelihood and impact of this risk.
            The score equals <strong>Likelihood × Impact</strong>.
          </Typography>

          {/* Matrix layout: rotated Y-label | grid */}
          <Box sx={{ display: "flex", gap: 1.5, alignItems: "stretch" }}>
            {/* Rotated LIKELIHOOD axis label */}
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                width: 20,
              }}
            >
              <Typography
                variant="caption"
                fontWeight={700}
                color="text.secondary"
                sx={{
                  writingMode: "vertical-rl",
                  transform: "rotate(180deg)",
                  letterSpacing: 2,
                  textTransform: "uppercase",
                  userSelect: "none",
                }}
              >
                Likelihood
              </Typography>
            </Box>

            {/* Grid area */}
            <Box sx={{ flex: 1 }}>
              {/* Column headers */}
              <Box
                sx={{
                  display: "grid",
                  gridTemplateColumns: "90px repeat(3, 1fr)",
                  gap: 1.5,
                  mb: 1.5,
                }}
              >
                <Box />
                {IMPACT_COLS.map((col) => (
                  <Typography
                    key={col.value}
                    variant="caption"
                    fontWeight={600}
                    color="text.secondary"
                    align="center"
                    sx={{ userSelect: "none" }}
                  >
                    {col.label}
                  </Typography>
                ))}
              </Box>

              {/* Data rows */}
              {LIKELIHOOD_ROWS.map((row) => (
                <Box
                  key={row.value}
                  sx={{
                    display: "grid",
                    gridTemplateColumns: "90px repeat(3, 1fr)",
                    gap: 0.75,
                    mb: 0.75,
                  }}
                >
                  {/* Row label */}
                  <Typography
                    variant="caption"
                    fontWeight={600}
                    color="text.secondary"
                    sx={{ display: "flex", alignItems: "center", userSelect: "none" }}
                  >
                    {row.label}
                  </Typography>

                  {/* Cells */}
                  {IMPACT_COLS.map((col) => {
                    const entry = findScore(row.value, col.value);
                    const isSelected = likelihood === row.value && impact === col.value;

                    return (
                      <Box
                        key={`${row.value}-${col.value}`}
                        component="button"
                        type="button"
                        onClick={() => handleCellClick(row.value, col.value)}
                        sx={{
                          height: 48,
                          borderRadius: 1.5,
                          bgcolor: entry?.color_code ?? "#ccc",
                          color: "#fff",
                          fontWeight: 700,
                          fontSize: "1rem",
                          cursor: "pointer",
                          border: "none",
                          outline: isSelected
                            ? "3px solid rgba(0,0,0,0.5)"
                            : "2px solid transparent",
                          boxShadow: isSelected
                            ? `inset 0 0 0 3px #fff, 0 2px 10px ${entry?.color_code ?? "#aaa"}88`
                            : "none",
                          transition: "filter 0.12s ease, transform 0.12s ease, box-shadow 0.12s ease",
                          "&:hover": {
                            filter: "brightness(0.85)",
                            transform: "scale(1.04)",
                          },
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                        }}
                      >
                        {entry?.risk_rating}
                      </Box>
                    );
                  })}
                </Box>
              ))}

              {/* IMPACT axis label */}
              <Typography
                variant="caption"
                fontWeight={700}
                color="text.secondary"
                align="center"
                sx={{
                  display: "block",
                  mt: 0.5,
                  letterSpacing: 2,
                  textTransform: "uppercase",
                  userSelect: "none",
                  pl: "90px",
                }}
              >
                Impact
              </Typography>
            </Box>
          </Box>

          {/* Validation error — fieldState.error ensures the Controller re-renders on trigger */}
          <Controller
            name="likelihood"
            control={control}
            rules={{
              validate: (val) =>
                val != null ? true : "Please select a cell in the risk matrix to continue",
            }}
            render={({ fieldState }) =>
              fieldState.error ? (
                <FormHelperText error>{fieldState.error.message}</FormHelperText>
              ) : <></>
            }
          />

          {/* Risk level chip — shown when a cell is selected */}
          {selectedScore && (
            <Stack direction="row" alignItems="center" gap={1.5} sx={{ flexWrap: "wrap" }}>
              <Typography variant="body2" color="text.secondary" fontWeight={500}>
                Gross Risk Score:
              </Typography>
              <Chip
                label={`${selectedScore.risk_rating} : ${selectedScore.risk_level} RISK`}
                size="medium"
                sx={{
                  bgcolor: selectedScore.color_code,
                  color: "#fff",
                  fontWeight: 700,
                  fontSize: "0.78rem",
                  px: 0.5,
                }}
              />
              <Typography variant="caption" color="text.secondary">
                (Likelihood {likelihood} × Impact {impact})
              </Typography>
            </Stack>
          )}
        </Stack>

        {/* ── Impact Description ────────────────────────────────────────────── */}
        <Controller
          name="impactDescription"
          control={control}
          rules={{ required: "Impact description is required" }}
          render={({ field, fieldState }) => (
            <TextField
              {...field}
              onChange={(e) => {
                field.onChange(e);
                if (e.target.value) clearErrors("impactDescription");
              }}
              label="Impact Description"
              fullWidth
              multiline
              rows={3}
              placeholder="Describe the specific consequences this risk could have on systems, data, or operations…"
              error={!!fieldState.error}
              helperText={fieldState.error?.message}
            />
          )}
        />

        {/* ── Timeline ─────────────────────────────────────────────────────── */}
        <Stack gap={3}>
          <SectionHeader title="Timeline" />

          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
              gap: 2,
              alignItems: "flex-start",
            }}
          >
            {/* Implementation Date */}
            <Controller
              name="implementationDate"
              control={control}
              rules={{ required: "Implementation date is required" }}
              render={({ field, fieldState }) => (
                <Box>
                  <FieldLabel>Implementation Date</FieldLabel>
                  <DatePicker
                    value={field.value}
                    onChange={(newValue) => {
                      field.onChange(newValue);
                      if (newValue) clearErrors("implementationDate");
                    }}
                    disablePast
                    sx={{ width: "100%" }}
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
                        error: !!fieldState.error,
                        helperText:
                          fieldState.error?.message ??
                          "Target completion date for risk treatment.",
                        onBlur: field.onBlur,
                      },
                    }}
                  />
                </Box>
              )}
            />

            {/* Reassessment Date */}
            <Controller
              name="reassessmentDate"
              control={control}
              rules={{ required: "Reassessment date is required" }}
              render={({ field, fieldState }) => (
                <Box>
                  <FieldLabel>Reassessment Date</FieldLabel>
                  <DatePicker
                    value={field.value}
                    onChange={(newValue) => {
                      field.onChange(newValue);
                      if (newValue) clearErrors("reassessmentDate");
                    }}
                    disablePast
                    sx={{ width: "100%" }}
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
                        error: !!fieldState.error,
                        helperText:
                          fieldState.error?.message ??
                          "Date for the next scheduled risk reassessment.",
                        onBlur: field.onBlur,
                      },
                    }}
                  />
                </Box>
              )}
            />
          </Box>
        </Stack>

      </Stack>
    </LocalizationProvider>
  );
}

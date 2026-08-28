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

import { Alert, Box, CircularProgress, Paper, Typography } from "@wso2/oxygen-ui";
import { type JSX, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { fetchRiskScores, type RiskScore } from "../api/adminApi";

// Orientation mirrors the Add Risk form matrix (RiskAssessmentStep): Likelihood
// Y-axis top (High 3) → bottom (Low 1), Impact X-axis left (Minor 1) → right (Major 3).
const LIKELIHOOD_ROWS = [
  { value: 3, label: "High 3" },
  { value: 2, label: "Medium 2" },
  { value: 1, label: "Low 1" },
] as const;

const IMPACT_COLS = [
  { value: 1, label: "Minor 1" },
  { value: 2, label: "Moderate 2" },
  { value: 3, label: "Major 3" },
] as const;

// Read-only, deliberately — no add/edit UI at all, not even for color. The
// 3x3 likelihood x impact matrix is a fixed set of 9 load-bearing constants
// (risk-level thresholds referenced throughout the risk workflow); free-form
// CRUD here could produce an invalid matrix with no natural validation.
export default function RiskScoresPage(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [scores, setScores] = useState<RiskScore[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchRiskScores(authFetch)
      .then(setScores)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load risk scores"))
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress size={22} />
      </Box>
    );
  }
  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  const cellFor = (likelihood: number, impact: number) =>
    scores.find((s) => s.likelihood === likelihood && s.impact === impact);

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        The fixed likelihood × impact matrix used to compute a risk's rating and level. Read-only; this table isn't
        editable from the Admin Console.
      </Typography>
      <Paper variant="outlined" sx={{ p: 2, maxWidth: 560 }}>
        <Box sx={{ display: "flex", gap: 1.5, alignItems: "stretch" }}>
          {/* Rotated Y-axis label */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", width: 20 }}>
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

          <Box sx={{ flex: 1 }}>
            {/* Column headers */}
            <Box sx={{ display: "grid", gridTemplateColumns: "90px repeat(3, 1fr)", gap: 0.75, mb: 1.5 }}>
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
                sx={{ display: "grid", gridTemplateColumns: "90px repeat(3, 1fr)", gap: 0.75, mb: 0.75 }}
              >
                <Typography
                  variant="caption"
                  fontWeight={600}
                  color="text.secondary"
                  sx={{ display: "flex", alignItems: "center", userSelect: "none" }}
                >
                  {row.label}
                </Typography>

                {IMPACT_COLS.map((col) => {
                  const cell = cellFor(row.value, col.value);
                  return (
                    <Box
                      key={col.value}
                      sx={{
                        height: 56,
                        borderRadius: 1.5,
                        bgcolor: cell?.color_code || "#ccc",
                        // Dark text on every cell: #fff drops below WCAG AA on the
                        // score colours (~2-4:1), worse here than in the Add Risk
                        // form because this cell also carries a 10px risk_level.
                        // The fallback #ccc is a fixed, theme-independent colour.
                        color: "#1a1a1a",
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "center",
                        justifyContent: "center",
                        userSelect: "none",
                      }}
                    >
                      <Box sx={{ fontWeight: 700, fontSize: "1rem", lineHeight: 1.2 }}>
                        {cell?.risk_rating ?? "—"}
                      </Box>
                      <Box sx={{ fontSize: 10, fontWeight: 700, letterSpacing: 0.5 }}>
                        {cell?.risk_level ?? ""}
                      </Box>
                    </Box>
                  );
                })}
              </Box>
            ))}

            {/* X-axis label */}
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
      </Paper>
    </Box>
  );
}

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

import { Box, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RiskScore } from "../../api/riskApi";
import { IMPACT_COLS, LIKELIHOOD_ROWS } from "../../riskMatrix";

interface RiskScoreGridProps {
  riskScores: RiskScore[];
  likelihood: number;
  impact: number;
  onChange: (likelihood: number, impact: number) => void;
  error?: string;
}

export default function RiskScoreGrid({
  riskScores,
  likelihood,
  impact,
  onChange,
  error,
}: RiskScoreGridProps): JSX.Element {
  const findScore = (l: number, i: number) =>
    riskScores.find((s) => s.likelihood === l && s.impact === i);

  const selected = findScore(likelihood, impact);

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1.5, alignItems: "stretch" }}>
        {/* Y-axis label */}
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
          <Box sx={{ display: "grid", gridTemplateColumns: "90px repeat(3, 1fr)", gap: 1.5, mb: 1.5 }}>
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
                const entry = findScore(row.value, col.value);
                const isSelected = likelihood === row.value && impact === col.value;

                return (
                  <Box
                    key={`${row.value}-${col.value}`}
                    component="button"
                    type="button"
                    aria-label={`Likelihood ${row.label}, Impact ${col.label}${entry ? `, rating ${entry.risk_rating} (${entry.risk_level})` : ""}`}
                    aria-pressed={isSelected}
                    onClick={() => onChange(row.value, col.value)}
                    sx={{
                      height: 48,
                      borderRadius: 1.5,
                      bgcolor: entry?.color_code ?? "#ccc",
                      color: "#fff",
                      fontWeight: 700,
                      fontSize: "1rem",
                      cursor: "pointer",
                      border: "none",
                      outline: "none",
                      boxShadow: isSelected
                        ? `inset 0 0 0 3px #fff, 0 2px 10px ${entry?.color_code ?? "#aaa"}88`
                        : "none",
                      transition: "filter 0.12s ease, transform 0.12s ease, box-shadow 0.12s ease",
                      "&:hover": { filter: "brightness(0.85)", transform: "scale(1.04)" },
                      "&:focus-visible": { outline: "3px solid rgba(0,0,0,0.6)", outlineOffset: 2 },
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

          {/* X-axis label */}
          <Typography
            variant="caption"
            fontWeight={700}
            color="text.secondary"
            align="center"
            sx={{ display: "block", mt: 0.5, letterSpacing: 2, textTransform: "uppercase", userSelect: "none", pl: "90px" }}
          >
            Impact
          </Typography>
        </Box>
      </Box>

      {selected && (
        <Box sx={{ mt: 1.5, display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              width: 12,
              height: 12,
              borderRadius: "50%",
              bgcolor: selected.color_code,
              flexShrink: 0,
            }}
          />
          <Typography variant="body2">
            <strong>{selected.risk_level}</strong> : Rating {selected.risk_rating}
          </Typography>
        </Box>
      )}

      {error && (
        <Typography variant="caption" color="error" sx={{ mt: 0.5, display: "block" }}>
          {error}
        </Typography>
      )}
    </Box>
  );
}

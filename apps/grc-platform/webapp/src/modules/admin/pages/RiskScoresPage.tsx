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

  const levels = [1, 2, 3];
  const cellFor = (likelihood: number, impact: number) =>
    scores.find((s) => s.likelihood === likelihood && s.impact === impact);

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        The fixed likelihood × impact matrix used to compute a risk's rating and level. Read-only — this table isn't
        editable from the Admin Console.
      </Typography>
      <Paper variant="outlined" sx={{ p: 2, maxWidth: 480 }}>
        <Box sx={{ display: "grid", gridTemplateColumns: "auto repeat(3, 1fr)", gap: 1 }}>
          <Box />
          {levels.map((impact) => (
            <Typography key={impact} variant="caption" fontWeight={700} textAlign="center">
              Impact {impact}
            </Typography>
          ))}
          {levels.map((likelihood) => (
            <Box key={likelihood} sx={{ display: "contents" }}>
              <Typography variant="caption" fontWeight={700} sx={{ alignSelf: "center" }}>
                Likelihood {likelihood}
              </Typography>
              {levels.map((impact) => {
                const cell = cellFor(likelihood, impact);
                return (
                  <Box
                    key={impact}
                    sx={{
                      borderRadius: 1,
                      p: 1,
                      textAlign: "center",
                      bgcolor: cell?.color_code || "action.hover",
                      color: "#1a1a1a",
                      fontSize: 12,
                    }}
                  >
                    <div>{cell?.risk_rating ?? "—"}</div>
                    <div style={{ fontSize: 10, fontWeight: 700 }}>{cell?.risk_level ?? ""}</div>
                  </Box>
                );
              })}
            </Box>
          ))}
        </Box>
      </Paper>
    </Box>
  );
}

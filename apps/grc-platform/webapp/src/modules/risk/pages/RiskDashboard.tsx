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

import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import {
  fetchDashboard,
  fetchRiskScores,
  fetchSourceRegisterTeams,
  type DashboardSummary,
  type RiskScore,
  type RiskTeam,
} from "../api/riskApi";
import DashboardView from "./dashboard/DashboardView";
import RegisterFilter from "./analytics/RegisterFilter";

// Risk dashboard: current organisational risk posture built from a single
// GET /api/v1/risks/dashboard payload, plus the 3×3 risk_score matrix that
// colors heatmap cells holding no risks. Optionally scoped by the same
// register filter used on the Analytics page.
export default function RiskDashboard(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [teams, setTeams] = useState<RiskTeam[]>([]);
  const [registerId, setRegisterId] = useState(0); // 0 = All Registers
  const [dashboard, setDashboard] = useState<DashboardSummary | null>(null);
  const [scores, setScores] = useState<RiskScore[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchSourceRegisterTeams(authFetch).then(setTeams).catch(console.error);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const load = useCallback(async (): Promise<void> => {
    setLoading(true);
    setError(null);
    try {
      const [summary, scoreMatrix] = await Promise.all([
        fetchDashboard(authFetch, registerId || undefined),
        fetchRiskScores(authFetch).catch(() => [] as RiskScore[]),
      ]);
      setDashboard(summary);
      setScores(scoreMatrix);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load the dashboard.");
    } finally {
      setLoading(false);
    }
  }, [authFetch, registerId]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start" flexWrap="wrap" gap={2}>
        <Box>
          <Typography variant="h4" fontWeight={700}>
            Risk Dashboard
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Overview of organizational risk posture
          </Typography>
        </Box>
        <RegisterFilter teams={teams} value={registerId} onChange={setRegisterId} />
      </Stack>

      {loading && (
        <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}>
          <CircularProgress />
        </Box>
      )}

      {!loading && error && (
        <Alert
          severity="error"
          action={
            <Button color="inherit" size="small" onClick={() => void load()}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      )}

      {!loading && !error && dashboard && (
        <DashboardView dashboard={dashboard} scores={scores} isAllRegisters={registerId === 0} />
      )}
    </Box>
  );
}

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
import { Alert, Box, Button, CircularProgress, Stack, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import {
  fetchAnalytics,
  fetchSourceRegisterTeams,
  type AnalyticsSummary,
  type RiskTeam,
} from "../api/riskApi";
import AnalyticsView from "./analytics/AnalyticsView";
import RegisterFilter from "./analytics/RegisterFilter";
import CsvExportButton from "./analytics/CsvExportButton";

// Risk Analytics: trend/time-series and cross-cutting metrics built from a
// single GET /api/v1/risks/analytics/summary payload, scoped by an optional
// register filter. Deliberately complements rather than duplicates the
// point-in-time breakdowns already on the Risk Dashboard.
export default function RiskAnalytics(): JSX.Element {
  const authFetch = useAuthApiClient();
  const [teams, setTeams] = useState<RiskTeam[]>([]);
  const [registerId, setRegisterId] = useState(0); // 0 = All Registers
  const [analytics, setAnalytics] = useState<AnalyticsSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchSourceRegisterTeams(authFetch).then(setTeams).catch(console.error);
  }, []);

  const load = useCallback(async (): Promise<void> => {
    setLoading(true);
    setError(null);
    try {
      const summary = await fetchAnalytics(authFetch, registerId || undefined);
      setAnalytics(summary);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load analytics.");
    } finally {
      setLoading(false);
    }
  }, [registerId]);

  useEffect(() => {
    void load();
  }, [load]);

  const registerLabel = registerId
    ? (teams.find((t) => t.id === registerId)?.name ?? "register").replace(/\s+/g, "-").toLowerCase()
    : "all-registers";

  return (
    <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start" flexWrap="wrap" gap={2}>
        <Box>
          <Typography variant="h4" fontWeight={700}>
            Risk Analytics
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Trends, cross-cutting metrics, and key risk indicators over time
          </Typography>
        </Box>
        <Stack direction="row" gap={1.5} alignItems="center">
          <RegisterFilter teams={teams} value={registerId} onChange={setRegisterId} />
          <CsvExportButton data={analytics} registerLabel={registerLabel} />
        </Stack>
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

      {!loading && !error && analytics && (
        <AnalyticsView analytics={analytics} isAllRegisters={registerId === 0} />
      )}
    </Box>
  );
}

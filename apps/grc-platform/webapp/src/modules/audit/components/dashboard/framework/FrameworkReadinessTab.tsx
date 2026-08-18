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

import { useQueries } from "@tanstack/react-query";
import { Alert, Box, Button, Chip, Paper, Skeleton, Typography } from "@wso2/oxygen-ui";
import type { JSX, ReactNode } from "react";
import { useMemo, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import EmptyState from "@components/empty-state/EmptyState";
import { useGetFrameworks } from "@modules/audit/api/useGetFrameworks";
import { controlsQueryKey } from "@modules/audit/api/useGetControls";
import type { Audit, AuditControl, ControlListResponse } from "@modules/audit/types/audit";
import type { ScopeRollup } from "@modules/audit/types/framework";
import { computeFrameworkRollups } from "@modules/audit/utils/frameworkRollup";
import FrameworkCard from "./FrameworkCard";
import AuditStrip from "./AuditStrip";
import StandPanel from "./StandPanel";
import BlockerList from "./BlockerList";
import FrameworkTeamBreakdown from "./FrameworkTeamBreakdown";

const RAIL_COLLAPSE_THRESHOLD = 6;

function PanelCard({ title, children }: { title: string; children: ReactNode }): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden", height: "100%", display: "flex", flexDirection: "column" }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: 1, borderColor: "divider" }}>
        <Typography variant="subtitle1" fontWeight={700}>{title}</Typography>
      </Box>
      <Box sx={{ p: 2.5, flex: 1, minHeight: 0 }}>{children}</Box>
    </Paper>
  );
}

interface FrameworkReadinessTabProps {
  audits: Audit[];
}

export default function FrameworkReadinessTab({ audits }: FrameworkReadinessTabProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const { data: allFrameworks } = useGetFrameworks();

  const [selectedFrameworkId, setSelectedFrameworkId] = useState<number | null>(null);
  const [selectedAuditId, setSelectedAuditId] = useState<number | null>(null);
  const [showAll, setShowAll] = useState(false);

  // Rail-level rollups need no per-audit fetch (§6) — only Audit.controlCounts.
  const railRollups = useMemo(() => computeFrameworkRollups(audits), [audits]);

  const effectiveFrameworkId = selectedFrameworkId ?? railRollups[0]?.id ?? null;
  const selectedRailFramework = railRollups.find((r) => r.id === effectiveFrameworkId) ?? null;
  const selectedAuditIds = useMemo(
    () => selectedRailFramework?.audits.map((a) => a.id) ?? [],
    [selectedRailFramework],
  );

  // Detail (exact phase/blocker/team breakdown) is fetched only for the
  // selected framework's audits — the rail above never waits on this (§6).
  const controlQueries = useQueries({
    queries: selectedAuditIds.map((auditId) => ({
      queryKey: controlsQueryKey(auditId),
      queryFn: async (): Promise<ControlListResponse> => {
        const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls`);
        if (!res.ok) throw new Error(`Failed to load controls (${res.status})`);
        return res.json() as Promise<ControlListResponse>;
      },
    })),
  });

  const controlsByAuditId = useMemo(() => {
    const map: Record<number, AuditControl[]> = {};
    controlQueries.forEach((q, i) => {
      if (q.data) map[selectedAuditIds[i]] = q.data.items;
    });
    return map;
  }, [controlQueries, selectedAuditIds]);

  // Gates the detail panels below: until every selected audit's controls
  // query has resolved, the framework's rollup is a mix of exact and
  // rail-only (coarse) figures — showing that as if it were the finished
  // breakdown would misrepresent partial data as complete (see
  // computeFrameworkRollups' hasDetail, which is now `every`, not `some`).
  const controlsLoading = controlQueries.some((q) => q.isLoading);
  const controlsError = controlQueries.some((q) => q.isError);

  const rollups = useMemo(
    () => computeFrameworkRollups(audits, controlsByAuditId),
    [audits, controlsByAuditId],
  );

  const selectedFramework = rollups.find((r) => r.id === effectiveFrameworkId) ?? null;
  const scope: ScopeRollup | null = selectedFramework
    ? (selectedAuditId !== null
        ? (selectedFramework.audits.find((a) => a.id === selectedAuditId) ?? selectedFramework)
        : selectedFramework)
    : null;

  const inactiveFrameworkCount = useMemo(() => {
    if (!allFrameworks) return 0;
    const activeIds = new Set(rollups.map((r) => r.id));
    return allFrameworks.filter((f) => !activeIds.has(f.id)).length;
  }, [allFrameworks, rollups]);

  if (rollups.length === 0) {
    return (
      <EmptyState
        title="No active frameworks"
        message="Frameworks appear here once they have at least one active audit."
      />
    );
  }

  const visibleRollups = showAll ? rollups : rollups.slice(0, RAIL_COLLAPSE_THRESHOLD);

  function selectFramework(id: number) {
    setSelectedFrameworkId(id);
    setSelectedAuditId(null);
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Box
        role="tablist"
        aria-label="Frameworks"
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr", lg: "1fr 1fr 1fr" },
          gap: 2,
        }}
      >
        {visibleRollups.map((rollup) => (
          <FrameworkCard
            key={rollup.id}
            rollup={rollup}
            selected={rollup.id === effectiveFrameworkId}
            onSelect={() => selectFramework(rollup.id)}
          />
        ))}
      </Box>

      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
        {rollups.length > RAIL_COLLAPSE_THRESHOLD && !showAll && (
          <Button size="small" variant="outlined" onClick={() => setShowAll(true)}>
            Show all ({rollups.length})
          </Button>
        )}
        {inactiveFrameworkCount > 0 && (
          <Chip variant="outlined" size="small" label={`+${inactiveFrameworkCount} frameworks with no active audit`} />
        )}
      </Box>

      {selectedFramework && scope && (
        <>
          <AuditStrip
            framework={selectedFramework}
            selectedAuditId={selectedAuditId}
            onSelect={setSelectedAuditId}
          />

          {controlsError ? (
            <Alert severity="error">Failed to load control details. Please refresh the page.</Alert>
          ) : controlsLoading ? (
            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr 1fr" }, gap: 2, height: 420 }}>
              <Skeleton variant="rectangular" height="100%" sx={{ borderRadius: 2 }} />
              <Skeleton variant="rectangular" height="100%" sx={{ borderRadius: 2 }} />
              <Skeleton variant="rectangular" height="100%" sx={{ borderRadius: 2 }} />
            </Box>
          ) : (
            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr 1fr" }, gap: 2, height: 420 }}>
              <PanelCard title="Where we stand">
                <StandPanel scope={scope} />
              </PanelCard>
              <PanelCard title="What's blocking">
                <BlockerList scope={scope} showAuditLabel={selectedAuditId === null} />
              </PanelCard>
              <PanelCard title="Who owns the gap">
                <FrameworkTeamBreakdown scope={scope} />
              </PanelCard>
            </Box>
          )}
        </>
      )}
    </Box>
  );
}

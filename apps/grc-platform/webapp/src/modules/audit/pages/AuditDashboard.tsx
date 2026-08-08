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

import { Alert, Box, Paper, Skeleton, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import { useGetDashboard } from "@modules/audit/api/useGetDashboard";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import { useIdTokenClaims } from "@hooks/useIdTokenClaims";
import TabBar, { type TabOption } from "@components/tab-bar/TabBar";
import AuditProgressList from "@modules/audit/components/dashboard/AuditProgressList";
import HeroBand from "@modules/audit/components/dashboard/HeroBand";
import KpiCards from "@modules/audit/components/dashboard/KpiCards";
import PhaseDonut from "@modules/audit/components/dashboard/PhaseDonut";
import PhaseInsightDialog from "@modules/audit/components/dashboard/PhaseInsightDialog";
import TeamProgress from "@modules/audit/components/dashboard/TeamProgress";
import TeamInsightDialog from "@modules/audit/components/dashboard/TeamInsightDialog";
import WorkQueue, {
  QUEUE_TAB_AWAITING,
  QUEUE_TAB_OVERDUE,
} from "@modules/audit/components/dashboard/WorkQueue";
import FrameworkReadinessTab from "@modules/audit/components/dashboard/framework/FrameworkReadinessTab";
import type { ControlPhase } from "@modules/audit/utils/controlStatus";
import type { TeamCompletion } from "@modules/audit/types/dashboard";

const TAB_OVERVIEW = "overview";
const TAB_FRAMEWORKS = "frameworks";

// ── Section card ──────────────────────────────────────────────────────────────

function SectionCard({ title, children }: { title: string; children: React.ReactNode }): JSX.Element {
  return (
    // height: 100% + flex column lets grid rows stretch all cards equally;
    // children with flex-basis 0 (AuditProgressList/TeamProgress) then fill
    // whatever height the tallest card (e.g. the detailed donut) sets.
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden", height: "100%", display: "flex", flexDirection: "column" }}>
      <Box sx={{ px: 2.5, py: 1.5, borderBottom: 1, borderColor: "divider" }}>
        <Typography variant="subtitle1" fontWeight={700}>{title}</Typography>
      </Box>
      <Box sx={{ p: 2.5, flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>{children}</Box>
    </Paper>
  );
}

// ── Skeleton ──────────────────────────────────────────────────────────────────

function DashboardSkeleton(): JSX.Element {
  return (
    <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>
      <Skeleton variant="rectangular" height={140} sx={{ borderRadius: 2 }} />
      <Box sx={{ display: "flex", gap: 2 }}>
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} variant="rectangular" height={92} sx={{ borderRadius: 2, flex: 1 }} />
        ))}
      </Box>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr 1fr" }, gap: 2 }}>
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
        <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 2 }} />
      </Box>
      <Skeleton variant="rectangular" height={260} sx={{ borderRadius: 2 }} />
    </Box>
  );
}

// ── Main ──────────────────────────────────────────────────────────────────────

export default function AuditDashboard(): JSX.Element {
  const { can } = useAuditPrivileges();
  const { data, isLoading, isError } = useGetDashboard();
  const { data: auditsData } = useGetAudits();
  const claims = useIdTokenClaims();

  // Tab state lives in the URL (?tab=frameworks) so a status email can link
  // straight to the Frameworks view; Overview is the default landing tab.
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = searchParams.get("tab") === TAB_FRAMEWORKS ? TAB_FRAMEWORKS : TAB_OVERVIEW;
  const setActiveTab = (tabId: string) => {
    const next = new URLSearchParams(searchParams);
    if (tabId === TAB_FRAMEWORKS) next.set("tab", TAB_FRAMEWORKS);
    else next.delete("tab");
    setSearchParams(next, { replace: true });
  };

  const tabs: TabOption[] = [
    { id: TAB_OVERVIEW, label: "Overview" },
    { id: TAB_FRAMEWORKS, label: "Frameworks" },
  ];

  const queueRef = useRef<HTMLDivElement>(null);
  const [queueTab, setQueueTab] = useState(QUEUE_TAB_AWAITING);
  const [queueHighlight, setQueueHighlight] = useState(false);

  // Chart drill-downs.
  const [selectedPhase, setSelectedPhase] = useState<ControlPhase | null>(null);
  const [selectedTeam, setSelectedTeam] = useState<{ team: TeamCompletion; color: string } | null>(null);

  const jumpToQueue = (tab: number) => {
    setQueueTab(tab);
    queueRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    setQueueHighlight(true);
    setTimeout(() => setQueueHighlight(false), 1800);
  };

  if (isLoading) return <DashboardSkeleton />;

  if (isError || !data) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">Failed to load dashboard. Please refresh the page.</Alert>
      </Box>
    );
  }

  const { auditStats, stats, statusDistribution, teamCompletion } = data;

  // Privilege-driven gating (never role names).
  const canSubmit = can(AuditPrivilege.SubmitEvidence);
  const canApprove = can(AuditPrivilege.ReviewEvidence);
  const hasQueue = canSubmit || canApprove;
  const queueTitle = canApprove ? "Review Queue" : canSubmit ? "My Tasks" : "Action Items";
  const awaitingCount = hasQueue ? (stats.totalActionItems ?? null) : null;

  const dueSoonCount = data.dueSoonItems?.length ?? 0;
  const pendingCount = data.pendingItems?.length ?? 0;
  const validationCount = data.validationItems?.length ?? 0;

  const userName =
    (claims?.given_name as string | undefined) ??
    (claims?.username as string | undefined)?.split("@")[0] ??
    null;

  return (
    <Box sx={{ p: 3, display: "flex", flexDirection: "column", gap: 3 }}>

      <TabBar tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} sx={{ mb: 0 }} />

      {activeTab === TAB_FRAMEWORKS ? (
        <FrameworkReadinessTab audits={auditsData?.items ?? []} />
      ) : (
        <>

      {/* ① Hero band — greeting, completion ring, attention chips */}
      <HeroBand
        userName={userName}
        completionPercent={stats.completionPercent}
        activeAudits={auditStats.activeAudits}
        totalControls={stats.totalControls}
        overdueCount={stats.overdueControls}
        dueSoonCount={dueSoonCount}
        awaitingCount={awaitingCount}
        awaitingLabel={canApprove ? "to review" : "to submit"}
        onOverdueClick={() => jumpToQueue(QUEUE_TAB_OVERDUE)}
        onQueueClick={() => jumpToQueue(QUEUE_TAB_AWAITING)}
      />

      {/* ② KPI cards */}
      <KpiCards
        totalControls={stats.totalControls}
        completedControls={stats.completedControls}
        completionPercent={stats.completionPercent}
        overdueControls={stats.overdueControls}
        awaitingCount={awaitingCount}
        awaitingLabel={queueTitle}
        onAwaitingClick={() => jumpToQueue(QUEUE_TAB_AWAITING)}
        onOverdueClick={() => jumpToQueue(QUEUE_TAB_OVERDUE)}
      />

      {/* ③ Charts row */}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr 1fr" }, gap: 2 }}>
        <SectionCard title="Controls by Phase">
          <PhaseDonut data={statusDistribution} onPhaseClick={setSelectedPhase} />
        </SectionCard>
        <SectionCard title="Audit Progress">
          <AuditProgressList audits={auditsData?.items ?? []} />
        </SectionCard>
        <SectionCard title="Team Progress">
          <TeamProgress
            data={teamCompletion}
            onTeamClick={(team, color) => setSelectedTeam({ team, color })}
          />
        </SectionCard>
      </Box>

      {/* ④ Work queue — tabbed (awaiting you / due soon / overdue). Also shown
          read-only for viewers without submit/review since the overdue tab is
          the monitoring view (their awaiting tab will simply be empty). */}
      <Box
        ref={queueRef}
        sx={{
          borderRadius: 2,
          outline: queueHighlight ? "2px solid #FB8C00" : "2px solid transparent",
          transition: "outline-color 0.3s",
        }}
      >
        <SectionCard title="Work Queue">
          <WorkQueue
            totalActionItems={stats.totalActionItems ?? 0}
            totalDueSoonItems={dueSoonCount}
            totalPendingItems={pendingCount}
            totalValidationItems={validationCount}
            totalOverdueControls={stats.overdueControls}
            canApprove={canApprove}
            canSubmit={canSubmit}
            queueTitle={queueTitle}
            tab={queueTab}
            onTabChange={setQueueTab}
          />
        </SectionCard>
      </Box>

      {/* Drill-down dialogs */}
      <PhaseInsightDialog
        phase={selectedPhase}
        statusDistribution={statusDistribution}
        onClose={() => setSelectedPhase(null)}
      />
      <TeamInsightDialog
        team={selectedTeam?.team ?? null}
        color={selectedTeam?.color ?? "#1E88E5"}
        teamStatusDistribution={data.teamStatusDistribution ?? []}
        onClose={() => setSelectedTeam(null)}
      />

        </>
      )}

    </Box>
  );
}

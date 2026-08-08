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
import { useNavigate } from "react-router";
import type { BlockerReason, ScopeRollup } from "@modules/audit/types/framework";
import { DUE_OVERDUE, DUE_SOON } from "@modules/audit/components/dashboard/dueDate";

const REASON_LABELS: Record<BlockerReason, string> = {
  overdue: "Overdue",
  needsClarification: "Needs Clarification",
  dueSoon: "Due Soon",
};

const REASON_COLORS: Record<BlockerReason, string> = {
  overdue: DUE_OVERDUE,
  needsClarification: DUE_SOON,
  dueSoon: DUE_SOON,
};

interface BlockerListProps {
  scope: ScopeRollup;
  /** Show each row's originating audit — only meaningful in the "All audits" view. */
  showAuditLabel: boolean;
}

// "What's blocking" — ranked overdue, then needs-clarification, then due
// within 7 days (§3.3). scope.blockers already arrives in that order.
export default function BlockerList({ scope, showAuditLabel }: BlockerListProps): JSX.Element {
  const navigate = useNavigate();
  const overdueCount = scope.blockers.filter((b) => b.reason === "overdue").length;
  const needsClarificationCount = scope.blockers.filter((b) => b.reason === "needsClarification").length;
  const dueSoonCount = scope.blockers.filter((b) => b.reason === "dueSoon").length;

  if (scope.blockers.length === 0) {
    return (
      <Box sx={{ height: 200, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <Typography variant="body2" color="text.secondary">Nothing blocking progress</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, height: "100%" }}>
      <Typography variant="caption" color="text.secondary">
        {overdueCount} overdue · {needsClarificationCount} needs clarification · {dueSoonCount} due soon
      </Typography>

      <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5, flex: 1, minHeight: 0, overflowY: "auto" }}>
        {scope.blockers.map((blocker) => (
          <Box
            key={blocker.controlId}
            role="button"
            tabIndex={0}
            onClick={() => void navigate(`/audit/audits/${blocker.auditId}?control=${blocker.controlId}`)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                void navigate(`/audit/audits/${blocker.auditId}?control=${blocker.controlId}`);
              }
            }}
            sx={{
              display: "flex", alignItems: "center", gap: 1,
              borderRadius: 1.5, px: 1, py: 0.75, cursor: "pointer",
              "&:hover": { bgcolor: "action.hover" },
            }}
          >
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography variant="body2" fontWeight={700} noWrap>
                {blocker.controlNumber}
                {showAuditLabel && (
                  <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                    {blocker.auditName}
                  </Typography>
                )}
              </Typography>
              <Typography variant="caption" color="text.secondary" noWrap>
                {blocker.team} · {blocker.owner}
              </Typography>
            </Box>
            <Typography variant="caption" fontWeight={700} sx={{ color: REASON_COLORS[blocker.reason], flexShrink: 0, whiteSpace: "nowrap" }}>
              {REASON_LABELS[blocker.reason]}
            </Typography>
          </Box>
        ))}
      </Box>
    </Box>
  );
}

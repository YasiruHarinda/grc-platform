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

import {
  Box,
  Card,
  CardActionArea,
  CardContent,
  Chip,
  Divider,
  IconButton,
  LinearProgress,
  ListItemIcon,
  Menu,
  MenuItem,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Archive, ArchiveRestore, CalendarDays, MoreVertical, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useState } from "react";
import AuditStatusChip from "@modules/audit/components/AuditStatusChip";
import CompletionRing from "@modules/audit/components/dashboard/CompletionRing";
import { daysLeft, formatDateRange } from "@modules/audit/utils/format";
import type { Audit, AuditStatus } from "@modules/audit/types/audit";

// Status accent colors — match AuditStatusChip's palette intent.
const STATUS_ACCENT: Record<AuditStatus, string> = {
  ACTIVE:    "#2E7D32",
  COMPLETED: "#1565C0",
  ARCHIVED:  "#757575",
  REMOVED:   "#757575",
};

const ENDING_SOON_DAYS = 14;

interface AuditCardProps {
  audit: Audit;
  onClick: () => void;
  onDelete: () => void;
  canDelete?: boolean;
  /** Archives an active/completed audit, or restores an archived one to active. */
  onArchiveToggle?: () => void;
  canArchive?: boolean;
}

export default function AuditCard({
  audit,
  onClick,
  onDelete,
  canDelete = false,
  onArchiveToggle,
  canArchive = false,
}: AuditCardProps): JSX.Element {
  const { controlCounts } = audit;
  const approvedPct =
    controlCounts.total > 0
      ? Math.round((controlCounts.approved / controlCounts.total) * 100)
      : 0;

  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);

  const handleMenuOpen = (e: React.MouseEvent<HTMLElement>) => {
    setMenuAnchor(e.currentTarget);
  };
  const handleDelete = () => {
    setMenuAnchor(null);
    onDelete();
  };
  const handleArchiveToggle = () => {
    setMenuAnchor(null);
    onArchiveToggle?.();
  };

  const showMenu = canDelete || canArchive;
  const isArchived = audit.status === "ARCHIVED";

  // Days-left hint for active audits (red when the period is nearly over).
  const remaining = audit.status === "ACTIVE" ? daysLeft(audit.periodEnd) : null;
  const endingSoon = remaining !== null && remaining >= 0 && remaining <= ENDING_SOON_DAYS;

  return (
    <Card
      variant="outlined"
      sx={{
        height: "100%",
        position: "relative",
        borderLeft: `3px solid ${STATUS_ACCENT[audit.status]}`,
        transition: "box-shadow 0.2s",
        "&:hover": { boxShadow: 4 },
      }}
    >
      {/* Menu button — only visible to users with archive/delete privileges */}
      {showMenu && (
        <>
          <Box sx={{ position: "absolute", top: 8, right: 8, zIndex: 1 }}>
            <IconButton size="small" aria-label="Audit actions" onClick={handleMenuOpen} sx={{ color: "text.secondary" }}>
              <MoreVertical size={16} />
            </IconButton>
          </Box>
          <Menu
            anchorEl={menuAnchor}
            open={Boolean(menuAnchor)}
            onClose={() => setMenuAnchor(null)}
          >
            {canArchive && (
              <MenuItem onClick={handleArchiveToggle}>
                <ListItemIcon>
                  {isArchived ? <ArchiveRestore size={16} /> : <Archive size={16} />}
                </ListItemIcon>
                {isArchived ? "Restore to active" : "Archive audit"}
              </MenuItem>
            )}
            {canDelete && (
              <MenuItem onClick={handleDelete} sx={{ color: "error.main" }}>
                <ListItemIcon sx={{ color: "error.main" }}>
                  <Trash2 size={16} />
                </ListItemIcon>
                Delete audit
              </MenuItem>
            )}
          </Menu>
        </>
      )}

      <CardActionArea
        onClick={onClick}
        sx={{ height: "100%", alignItems: "flex-start" }}
      >
        <CardContent sx={{ height: "100%", display: "flex", flexDirection: "column", gap: 1.5, p: 2.5 }}>
          {/* Top row: status chip + completion ring — leave space for the menu button */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pr: showMenu ? 3.5 : 0 }}>
            <AuditStatusChip status={audit.status} />
            <CompletionRing percent={approvedPct} size={40} caption={false} />
          </Box>

          {/* Audit name */}
          <Typography
            variant="h6"
            sx={{ fontWeight: 600, lineHeight: 1.3, mt: -0.5 }}
          >
            {audit.name}
          </Typography>

          {/* Framework · Product */}
          <Typography variant="body2" color="text.secondary">
            {audit.framework.name}
            {" · "}
            {audit.product.name}
          </Typography>

          {/* Period + days-left urgency hint */}
          <Stack direction="row" spacing={0.75} alignItems="center" flexWrap="wrap">
            <CalendarDays size={14} style={{ color: "var(--mui-palette-text-secondary, #666)" }} />
            <Typography variant="caption" color="text.secondary">
              {formatDateRange(audit.periodStart, audit.periodEnd)}
            </Typography>
            {remaining !== null && remaining >= 0 && (
              <Typography
                variant="caption"
                sx={{ fontWeight: endingSoon ? 700 : 400, color: endingSoon ? "error.main" : "text.secondary" }}
              >
                · {remaining === 0 ? "ends today" : `${remaining}d left`}
              </Typography>
            )}
          </Stack>

          <Divider sx={{ mt: "auto" }} />

          {/* Progress */}
          <Box>
            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                mb: 0.75,
              }}
            >
              <Typography variant="caption" color="text.secondary">
                {controlCounts.approved} / {controlCounts.total} approved
              </Typography>
              {controlCounts.overdue > 0 && (
                <Chip
                  icon={<AlertTriangle size={11} />}
                  label={`${controlCounts.overdue} overdue`}
                  size="small"
                  sx={{
                    height: 20, fontSize: "0.68rem", fontWeight: 700,
                    color: "#E53935", bgcolor: "transparent", border: "1px solid #E53935",
                    "& .MuiChip-icon": { color: "#E53935" },
                  }}
                />
              )}
            </Box>
            <LinearProgress
              variant="determinate"
              value={approvedPct}
              color={controlCounts.overdue > 0 ? "warning" : "success"}
              sx={{ height: 6, borderRadius: 3 }}
            />
          </Box>
        </CardContent>
      </CardActionArea>
    </Card>
  );
}

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
  Chip,
  Divider,
  Drawer,
  IconButton,
  Stack,
  Tab,
  Tabs,
} from "@mui/material";
import { Box, Typography } from "@wso2/oxygen-ui";
import { FileUp, History, Info, X } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import ControlStatusChip from "@modules/audit/components/ControlStatusChip";
import UserAvatar from "@modules/audit/components/UserAvatar";
import { formatAuditDate } from "@modules/audit/utils/format";
import type { AuditControl } from "@modules/audit/types/audit";

interface ControlDrawerProps {
  control: AuditControl | null;
  open: boolean;
  onClose: () => void;
}

interface TabPanelProps {
  value: number;
  index: number;
  children: React.ReactNode;
}

function TabPanel({ value, index, children }: TabPanelProps): JSX.Element {
  return (
    <Box
      role="tabpanel"
      hidden={value !== index}
      sx={{ flex: 1, overflowY: "auto", p: 2.5 }}
    >
      {value === index && children}
    </Box>
  );
}

function DetailRow({
  label,
  value,
}: {
  label: string;
  value: string | null | undefined;
}): JSX.Element {
  return (
    <Box sx={{ display: "flex", gap: 2, alignItems: "flex-start", py: 0.75 }}>
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ minWidth: 130, flexShrink: 0 }}
      >
        {label}
      </Typography>
      <Typography variant="body2">{value ?? "—"}</Typography>
    </Box>
  );
}

export default function ControlDrawer({
  control,
  open,
  onClose,
}: ControlDrawerProps): JSX.Element {
  const [tab, setTab] = useState(0);

  const REQ_TYPE_LABELS: Record<string, string> = {
    DESIGN: "Design",
    OE: "Operational Effectiveness",
  };
  const CTRL_TYPE_LABELS: Record<string, string> = {
    CONFIG: "Configuration",
    NON_CONFIG: "Non-Configuration",
  };
  const SCOPE_LABELS: Record<string, string> = {
    COMMON: "Common",
    PRODUCT_SPECIFIC: "Product Specific",
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: "100vw", sm: 520 },
          display: "flex",
          flexDirection: "column",
        },
      }}
    >
      {control && (
        <>
          {/* Header */}
          <Box sx={{ p: 2.5, pb: 1.5 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: 1,
                mb: 1,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                <Typography variant="h6" fontWeight={700}>
                  {control.controlNumber}
                </Typography>
                <ControlStatusChip status={control.status} />
                {control.isOverdue && (
                  <Chip label="Overdue" color="error" size="small" variant="outlined" />
                )}
              </Box>
              <IconButton size="small" onClick={onClose} aria-label="Close drawer">
                <X size={18} />
              </IconButton>
            </Box>
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{
                display: "-webkit-box",
                WebkitLineClamp: 2,
                WebkitBoxOrient: "vertical",
                overflow: "hidden",
              }}
            >
              {control.description}
            </Typography>
          </Box>

          <Divider />

          {/* Tabs */}
          <Tabs
            value={tab}
            onChange={(_, v: number) => setTab(v)}
            sx={{ px: 2, borderBottom: 1, borderColor: "divider" }}
          >
            <Tab icon={<Info size={15} />} iconPosition="start" label="Overview" />
            <Tab icon={<FileUp size={15} />} iconPosition="start" label="Evidence" />
            <Tab icon={<History size={15} />} iconPosition="start" label="History" />
          </Tabs>

          {/* Overview tab */}
          <TabPanel value={tab} index={0}>
            <Stack spacing={0.5}>
              <Typography variant="subtitle2" fontWeight={700} gutterBottom>
                Control Details
              </Typography>
              <DetailRow
                label="Requirement Type"
                value={REQ_TYPE_LABELS[control.requirementType]}
              />
              <DetailRow
                label="Control Type"
                value={CTRL_TYPE_LABELS[control.controlType]}
              />
              <DetailRow label="Scope" value={SCOPE_LABELS[control.scope]} />
              <DetailRow
                label="Due Date"
                value={control.dueDate ? formatAuditDate(control.dueDate) : null}
              />
              {/* Process Owner */}
              <Box sx={{ display: "flex", gap: 2, alignItems: "center", py: 0.75 }}>
                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 130, flexShrink: 0 }}>
                  Owner
                </Typography>
                {control.ownerName ? (
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <UserAvatar name={control.ownerName} size={28} />
                    <Typography variant="body2">{control.ownerName}</Typography>
                  </Box>
                ) : (
                  <Typography variant="body2" color="text.disabled">—</Typography>
                )}
              </Box>

              <DetailRow label="Team" value={control.teamName} />

              {/* Auditor POC */}
              <Box sx={{ display: "flex", gap: 2, alignItems: "center", py: 0.75 }}>
                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 130, flexShrink: 0 }}>
                  Auditor
                </Typography>
                {control.auditorName ? (
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <UserAvatar name={control.auditorName} size={28} />
                    <Typography variant="body2">{control.auditorName}</Typography>
                  </Box>
                ) : (
                  <Typography variant="body2" color="text.disabled">—</Typography>
                )}
              </Box>
              {control.sampleReference && (
                <DetailRow label="Sample Ref" value={control.sampleReference} />
              )}
            </Stack>

            {control.evidenceRequirement && (
              <Box sx={{ mt: 3 }}>
                <Divider sx={{ mb: 2 }} />
                <Typography variant="subtitle2" fontWeight={700} gutterBottom>
                  Evidence Requirement
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.7 }}>
                  {control.evidenceRequirement}
                </Typography>
              </Box>
            )}

            {control.comments && (
              <Box sx={{ mt: 3 }}>
                <Divider sx={{ mb: 2 }} />
                <Typography variant="subtitle2" fontWeight={700} gutterBottom>
                  Comments
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.7 }}>
                  {control.comments}
                </Typography>
              </Box>
            )}
          </TabPanel>

          {/* Evidence tab */}
          <TabPanel value={tab} index={1}>
            <Box
              sx={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                py: 8,
                gap: 2,
                textAlign: "center",
              }}
            >
              <FileUp size={40} style={{ opacity: 0.3 }} />
              <Typography variant="h6">Evidence Submission</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 320 }}>
                Azure Blob Storage integration coming soon. You will be able to
                upload and manage evidence files here.
              </Typography>
            </Box>
          </TabPanel>

          {/* History tab */}
          <TabPanel value={tab} index={2}>
            <Box
              sx={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                py: 8,
                gap: 2,
                textAlign: "center",
              }}
            >
              <History size={40} style={{ opacity: 0.3 }} />
              <Typography variant="h6">No history yet</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 320 }}>
                Audit trail entries will appear here as actions are taken on this
                control.
              </Typography>
            </Box>
          </TabPanel>
        </>
      )}
    </Drawer>
  );
}

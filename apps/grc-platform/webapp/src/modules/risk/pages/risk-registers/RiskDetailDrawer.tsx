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
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  Drawer,
  IconButton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import type * as React from "react";
import type { RiskDetail } from "../../api/riskApi";
import { RiskPrivilege } from "../../privileges";
import { STATUS_CONFIG, calcAge, calcDue, formatDate } from "./utils";

export interface DrawerActions {
  onOwnerApprove: () => void;
  onManagementApprove: () => void;
  onApprove: () => void;
  onReject: () => void;
  onComplete: () => void;
  onResubmit: () => void;
  onCloseRisk: () => void;
  onEdit: () => void;
  onAssess: () => void;
  onCancel: () => void;
}

interface RiskDetailDrawerProps extends DrawerActions {
  open: boolean;
  detail: RiskDetail | null;
  loading: boolean;
  error: string;
  actionsDisabled: boolean;
  can: (privilege: string) => boolean;
  onClose: () => void;
}

const REJECTION_STAGE_LABELS: Record<string, string> = {
  OWNER: "Risk Owner",
  MANAGEMENT: "Management",
  COMPLIANCE: "Compliance",
  COMPLETION_OWNER: "Risk Owner (Completion)",
};

function DetailRow({ label, value }: { label: string; value: React.ReactNode }): JSX.Element {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ display: "block" }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ mt: 0.25 }}>
        {value || "—"}
      </Typography>
    </Box>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }): JSX.Element {
  return (
    <Box sx={{ mt: 2.5, mb: 1.5 }}>
      <Typography variant="subtitle2" fontWeight={700} color="text.primary">
        {children}
      </Typography>
      <Divider sx={{ mt: 0.5 }} />
    </Box>
  );
}

function ActionFooter({
  status,
  actions,
  disabled,
  can,
}: {
  status: string;
  actions: DrawerActions;
  disabled: boolean;
  can: (privilege: string) => boolean;
}): JSX.Element | null {
  const rejectAndApprove = (approveLabel: string, onApprove: () => void, approvePriv: string, rejectPriv: string) => {
    const showReject = can(rejectPriv);
    const showApprove = can(approvePriv);
    if (!showReject && !showApprove) return null;
    return (
      <Box sx={{ display: "flex", gap: 1, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
        {showReject && (
          <Button variant="outlined" color="error" fullWidth disabled={disabled} onClick={actions.onReject}>
            Reject
          </Button>
        )}
        {showApprove && (
          <Button variant="contained" color="success" fullWidth disabled={disabled} onClick={onApprove}>
            {approveLabel}
          </Button>
        )}
      </Box>
    );
  };

  switch (status) {
    case "PENDING_RISK_OWNER_APPROVAL": {
      const showEdit = can(RiskPrivilege.UpdateRisk);
      const showCancel = can(RiskPrivilege.CancelRisk);
      const showReject = can(RiskPrivilege.OwnerRejectRisk);
      const showOwnerApprove = can(RiskPrivilege.OwnerApproveRisk);
      if (!showEdit && !showCancel && !showReject && !showOwnerApprove) return null;
      return (
        <Box sx={{ pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
          {(showEdit || showCancel) && (
            <Stack direction="row" gap={1} sx={{ mb: 1 }}>
              {showEdit && (
                <Button variant="outlined" fullWidth disabled={disabled} onClick={actions.onEdit}>
                  Edit Risk
                </Button>
              )}
              {showCancel && (
                <Button variant="outlined" color="error" fullWidth disabled={disabled} onClick={actions.onCancel}>
                  Cancel Risk
                </Button>
              )}
            </Stack>
          )}
          {(showReject || showOwnerApprove) && (
            <Box sx={{ display: "flex", gap: 1 }}>
              {showReject && (
                <Button variant="outlined" color="error" fullWidth disabled={disabled} onClick={actions.onReject}>
                  Reject
                </Button>
              )}
              {showOwnerApprove && (
                <Button variant="contained" color="success" fullWidth disabled={disabled} onClick={actions.onOwnerApprove}>
                  Approve as Risk Owner
                </Button>
              )}
            </Box>
          )}
        </Box>
      );
    }

    case "PENDING_AMENDMENT":
      return rejectAndApprove("Approve as Risk Owner", actions.onOwnerApprove, RiskPrivilege.OwnerApproveRisk, RiskPrivilege.OwnerRejectRisk);

    case "PENDING_MANAGEMENT_APPROVAL":
      return rejectAndApprove("Approve as Management", actions.onManagementApprove, RiskPrivilege.ManagementApproveRisk, RiskPrivilege.ManagementRejectRisk);

    case "PENDING_COMPLIANCE_REVIEW":
      return rejectAndApprove("Approve (Compliance)", actions.onApprove, RiskPrivilege.ComplianceApproveRisk, RiskPrivilege.ComplianceRejectRisk);

    case "PENDING_OWNER_COMPLETION_APPROVAL":
      return rejectAndApprove("Approve Completion", actions.onOwnerApprove, RiskPrivilege.OwnerApproveRisk, RiskPrivilege.OwnerRejectRisk);

    case "IN_REMEDIATION": {
      const showEdit = can(RiskPrivilege.UpdateRisk);
      const showAssess = can(RiskPrivilege.AssessRisk);
      const showComplete = can(RiskPrivilege.CompleteRisk);
      if (!showEdit && !showAssess && !showComplete) return null;
      return (
        <Box sx={{ pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
          {(showEdit || showAssess) && (
            <Stack direction="row" gap={1} sx={{ mb: 1 }}>
              {showEdit && (
                <Button variant="outlined" fullWidth disabled={disabled} onClick={actions.onEdit}>
                  Edit Risk
                </Button>
              )}
              {showAssess && (
                <Button variant="outlined" fullWidth disabled={disabled} onClick={actions.onAssess}>
                  Assess Risk
                </Button>
              )}
            </Stack>
          )}
          {showComplete && (
            <Button variant="contained" fullWidth disabled={disabled} onClick={actions.onComplete}>
              Submit for Approval
            </Button>
          )}
        </Box>
      );
    }

    case "PENDING_REVISION": {
      const showEdit = can(RiskPrivilege.UpdateRisk);
      const showResubmit = can(RiskPrivilege.SubmitRisk);
      if (!showEdit && !showResubmit) return null;
      return (
        <Box sx={{ pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
          <Stack direction="row" gap={1}>
            {showEdit && (
              <Button variant="outlined" fullWidth disabled={disabled} onClick={actions.onEdit}>
                Edit Risk
              </Button>
            )}
            {showResubmit && (
              <Button variant="contained" color="primary" fullWidth disabled={disabled} onClick={actions.onResubmit}>
                Resubmit
              </Button>
            )}
          </Stack>
        </Box>
      );
    }

    case "PENDING_COMPLIANCE_CLOSURE":
      if (!can(RiskPrivilege.CloseRisk)) return null;
      return (
        <Box sx={{ pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
          <Button variant="contained" fullWidth disabled={disabled} onClick={actions.onCloseRisk}>
            Close Risk
          </Button>
        </Box>
      );

    default:
      return null;
  }
}

export default function RiskDetailDrawer({
  open,
  detail,
  loading,
  error,
  actionsDisabled,
  can,
  onClose,
  ...actions
}: RiskDetailDrawerProps): JSX.Element {
  const status = detail?.workflow_status ?? "";
  const statusCfg = STATUS_CONFIG[status] ?? { label: status, color: "default" as const };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: "100%", sm: 800 },
          display: "flex",
          flexDirection: "column",
          p: 0,
          backdropFilter: "none",
          backgroundImage: "none",
          backgroundColor: "#ffffff",
          "[data-color-scheme='dark'] &": {
            backgroundColor: "#1a1a24",
          },
        },
      }}
    >
      {/* Fixed header */}
      <Box sx={{ px: 3, pt: 3, pb: 2, borderBottom: "1px solid", borderColor: "divider" }}>
        <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between" }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            {detail ? (
              <>
                <Typography variant="caption" color="text.secondary" fontWeight={600}>
                  {detail.risk_code}
                </Typography>
                <Typography variant="h6" fontWeight={700} sx={{ mt: 0.25, lineHeight: 1.3 }}>
                  {detail.risk_title}
                </Typography>
                <Stack direction="row" gap={1} sx={{ mt: 1.5 }} flexWrap="wrap">
                  <Chip label={statusCfg.label} color={statusCfg.color} size="small" sx={statusCfg.sx} />
                  {(() => {
                    const current = detail.effective_score ?? detail.gross_score;
                    return (
                      current && (
                        <Chip
                          label={`${current.risk_level} : Score ${current.risk_rating}`}
                          size="small"
                          sx={{ bgcolor: current.color_code, color: "#fff", fontWeight: 700 }}
                        />
                      )
                    );
                  })()}
                  <Chip
                    label={`Age: ${calcAge(detail.created_at)} days`}
                    size="small"
                    variant="outlined"
                  />
                  {(() => {
                    const due = calcDue(detail.implementation_date);
                    return (
                      <Typography variant="caption" fontWeight={700} sx={{ color: due.color, alignSelf: "center" }}>
                        {due.label}
                      </Typography>
                    );
                  })()}
                </Stack>
              </>
            ) : (
              <Typography variant="h6" fontWeight={700}>
                Risk Details
              </Typography>
            )}
          </Box>
          <IconButton onClick={onClose} size="small" aria-label="Close risk details" sx={{ ml: 1, mt: -0.5, flexShrink: 0 }}>
            <X size={18} />
          </IconButton>
        </Box>
      </Box>

      {/* Scrollable content */}
      <Box sx={{ flex: 1, overflowY: "auto", px: 3, py: 2 }}>
        {loading ? (
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: 200 }}>
            <CircularProgress />
          </Box>
        ) : error ? (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error}
          </Alert>
        ) : detail ? (
          <>
            {detail.rejection_comment && (
              <Alert severity="error" sx={{ mb: 2 }}>
                <Typography variant="caption" fontWeight={700} display="block">
                  Rejected at:{" "}
                  {detail.rejection_stage
                    ? (REJECTION_STAGE_LABELS[detail.rejection_stage] ?? detail.rejection_stage)
                    : "—"}
                </Typography>
                {detail.rejection_comment}
              </Alert>
            )}

            <SectionTitle>Basic Information</SectionTitle>
            <Stack gap={2}>
              <DetailRow label="Source Register" value={detail.source_register_name} />
              <DetailRow label="Description" value={detail.risk_description} />
              <DetailRow label="Impact Description" value={detail.impact_description} />
              <DetailRow label="Risk Identified Date" value={formatDate(detail.risk_identified_date)} />
              <DetailRow
                label="Identified By"
                value={
                  detail.identified_by_user_name ??
                  detail.identified_by_name ??
                  detail.identified_by_type ??
                  "—"
                }
              />
              <Stack direction="row" gap={4}>
                <DetailRow label="Assigned By" value={detail.assigner_name} />
                <DetailRow label="Risk Owner" value={detail.owner_name} />
              </Stack>
              {detail.compliance_references.length > 0 && (
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    fontWeight={600}
                    display="block"
                  >
                    Compliance References
                  </Typography>
                  <Stack direction="row" flexWrap="wrap" gap={0.75} sx={{ mt: 0.75 }}>
                    {detail.compliance_references.map((ref) => (
                      <Chip key={ref.id} label={ref.name} size="small" variant="outlined" />
                    ))}
                  </Stack>
                </Box>
              )}
            </Stack>

            <SectionTitle>Risk Treatment</SectionTitle>
            <Stack gap={2}>
              <Stack direction="row" gap={4}>
                <DetailRow label="Assignment Team" value={detail.assignment_team_name} />
                <DetailRow label="Treatment Strategy" value={detail.treatment_strategy} />
              </Stack>
              <Stack direction="row" gap={4}>
                <DetailRow label="Implementation Date" value={formatDate(detail.implementation_date)} />
                <DetailRow label="Reassessment Date" value={formatDate(detail.reassessment_date)} />
              </Stack>
              <DetailRow label="Progress" value={detail.progress} />
              <DetailRow label="Email Subject" value={detail.email_subject} />
              {detail.git_issue_url && (
                <DetailRow
                  label="Git Issue URL"
                  value={
                    <a href={detail.git_issue_url} target="_blank" rel="noreferrer">
                      {detail.git_issue_url}
                    </a>
                  }
                />
              )}
              <DetailRow label="Remarks" value={detail.remarks} />
            </Stack>

            {detail.action_plan && (
              <>
                <SectionTitle>Action Plan</SectionTitle>
                <Stack gap={1.5}>
                  <DetailRow label="Description" value={detail.action_plan.description} />
                  <DetailRow label="Status" value={detail.action_plan.status} />
                  {detail.action_plan.steps.length > 0 && (
                    <Box>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        fontWeight={600}
                        display="block"
                      >
                        Steps
                      </Typography>
                      {detail.action_plan.steps.map((step) => (
                        <Box key={step.id} sx={{ display: "flex", gap: 1, mt: 0.75 }}>
                          <Typography
                            variant="body2"
                            color="text.secondary"
                            fontWeight={600}
                            sx={{ minWidth: 24 }}
                          >
                            {step.step_no}.
                          </Typography>
                          <Typography variant="body2">{step.description}</Typography>
                        </Box>
                      ))}
                    </Box>
                  )}
                </Stack>
              </>
            )}

            {detail.assessments.length > 0 && (
              <>
                <SectionTitle>Assessment History</SectionTitle>
                <Stack gap={2}>
                  {detail.assessments.map((a) => (
                    <Box
                      key={a.is_initial ? "initial" : a.id}
                      sx={{
                        border: "1px solid",
                        borderColor: "divider",
                        borderRadius: 1,
                        p: 1.5,
                      }}
                    >
                      <Stack direction="row" justifyContent="space-between" alignItems="center">
                        <Chip
                          label={`${a.residual_level} : Score ${a.residual_rating}`}
                          size="small"
                          sx={{ bgcolor: a.residual_color_code, color: "#fff", fontWeight: 700 }}
                        />
                        <Typography variant="caption" color="text.secondary">
                          {formatDate(a.reassessment_date)}
                        </Typography>
                      </Stack>
                      {a.is_initial ? (
                        <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>
                          Initial assessment (gross score)
                        </Typography>
                      ) : (
                        <>
                          <Typography variant="body2" sx={{ mt: 1 }}>
                            {a.progress}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            Assessed by {a.assessed_by}
                          </Typography>
                        </>
                      )}
                    </Box>
                  ))}
                </Stack>
              </>
            )}
          </>
        ) : null}
      </Box>

      {/* Fixed action footer */}
      {detail && !loading && !error && (
        <Box sx={{ px: 3, pb: 3, pt: 0 }}>
          <ActionFooter status={status} actions={actions} disabled={actionsDisabled} can={can} />
        </Box>
      )}
    </Drawer>
  );
}

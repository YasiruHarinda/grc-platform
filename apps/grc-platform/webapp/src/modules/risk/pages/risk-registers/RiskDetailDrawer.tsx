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

import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Drawer,
  IconButton,
  Paper,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Briefcase,
  Calendar,
  Check,
  FileText,
  Link as LinkIcon,
  ListChecks,
  MessageSquare,
  Shield,
  Tag,
  TrendingUp,
  Users,
  Wrench,
  X,
} from "@wso2/oxygen-ui-icons-react";
import type { JSX, ReactNode } from "react";
import type { ActionPlan, ActionPlanStep, Escalation, HistoryEntry, RiskDetail } from "../../api/riskApi";
import RiskHistoryTimeline from "./RiskHistoryTimeline";
import { RiskPrivilege } from "../../privileges";
import { dialogPaperSx } from "../cardStyles";
import { STATUS_CONFIG, calcAge, calcDue, formatDate } from "./utils";

// ActionPlan doesn't embed its steps (GET .../action-plans lists plans only;
// steps come from a separate GET .../action-plans/{planId}/steps call) — the
// parent page fetches both and merges them before passing down here.
export type ActionPlanWithSteps = ActionPlan & { steps: ActionPlanStep[] };

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
  // Adds a further action plan (Risk Assigner only).
  onAddActionPlan: () => void;
  // Answers an escalation with a comment, returning the risk to its assigner.
  onCommentEscalation: () => void;
  onEscalate: () => void;
}

interface RiskDetailDrawerProps extends DrawerActions {
  open: boolean;
  detail: RiskDetail | null;
  loading: boolean;
  error: string;
  actionsDisabled: boolean;
  // No `can` prop: this drawer derives its own from the risk's
  // effective_privileges. Passing the session-wide set in would reintroduce
  // exactly the bug that change fixed — see the comment in the component body.
  onClose: () => void;
  // Full action-plan list (STANDARD + MANAGEMENT) — separate from
  // detail.action_plan, which only ever embeds the STANDARD one.
  actionPlans: ActionPlanWithSteps[];
  // Fetched independently of `detail`/`error` above, so a failure loading
  // action plans doesn't blank out the rest of the drawer — only the Action
  // Plans tab shows this.
  actionPlansError: string;
  // Escalation history, newest first. Drives the banner and whether the
  // "Review Escalation" action is offered.
  escalations: Escalation[];
  // Full risk history, newest first — every workflow event and field edit.
  history: HistoryEntry[];
  historyError: string;
  currentUserId: number | null;
  // id → display_name, resolved at render time so the Action Owner label
  // stays correct even when the drawer opens before the users list finishes
  // loading (e.g. a dashboard deep-link).
  userNames: Map<number, string>;
  onCompleteStep: (planId: number, stepId: number) => void;
  onCompletePlan: (planId: number) => void;
}

const REJECTION_STAGE_LABELS: Record<string, string> = {
  OWNER: "Risk Owner",
  MANAGEMENT: "Management",
  COMPLIANCE: "Compliance",
  COMPLETION_OWNER: "Risk Owner (Completion)",
  COMPLETION_MANAGEMENT: "Management (Completion)",
};

// ── Shared visual building blocks (matching Audit's ControlDrawer.tsx —
// SectionCard/InfoTile/TabPanel there are file-local, not exported/shared
// anywhere, so this is the same per-file-duplication convention, not a
// regression) ──────────────────────────────────────────────────────────────

interface SectionCardProps {
  icon: ReactNode;
  iconColor?: string;
  iconBg?: string;
  title: string;
  headerExtra?: ReactNode;
  children: ReactNode;
}

function SectionCard({
  icon,
  iconColor = "#475569",
  iconBg = "#f1f5f9",
  title,
  headerExtra,
  children,
}: SectionCardProps): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden" }}>
      <Box
        sx={{
          px: 2.5,
          py: 1.5,
          display: "flex",
          alignItems: "center",
          gap: 1.25,
          borderBottom: 1,
          borderColor: "divider",
          bgcolor: "action.hover",
        }}
      >
        <Box
          sx={{
            width: 30,
            height: 30,
            borderRadius: 1.5,
            bgcolor: iconBg,
            color: iconColor,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
          }}
        >
          {icon}
        </Box>
        <Typography variant="subtitle2" fontWeight={700} sx={{ flex: 1 }}>
          {title}
        </Typography>
        {headerExtra}
      </Box>
      <Box sx={{ p: 2.5 }}>{children}</Box>
    </Paper>
  );
}

function InfoTile({ label, children }: { label: string; children: ReactNode }): JSX.Element {
  return (
    <Box
      sx={{
        px: 1.25,
        py: 1,
        borderRadius: 1,
        border: "1px solid",
        borderColor: "divider",
        bgcolor: "action.hover",
        display: "flex",
        flexDirection: "column",
        gap: 0.4,
      }}
    >
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500, fontSize: "0.67rem", lineHeight: 1 }}>
        {label}
      </Typography>
      <Typography variant="body2" fontWeight={600}>
        {children || "—"}
      </Typography>
    </Box>
  );
}

function InfoGrid({ children }: { children: ReactNode }): JSX.Element {
  return <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>{children}</Box>;
}

function TabPanel({ value, index, children }: { value: number; index: number; children: ReactNode }): JSX.Element {
  return (
    <Box role="tabpanel" hidden={value !== index} sx={{ display: value === index ? "block" : "none" }}>
      {value === index && <Stack gap={2}>{children}</Stack>}
    </Box>
  );
}

function EmptyState({ icon, title, caption }: { icon: ReactNode; title: string; caption: string }): JSX.Element {
  return (
    <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", py: 6, gap: 1.5 }}>
      <Box
        sx={{
          width: 64,
          height: 64,
          borderRadius: "50%",
          bgcolor: "action.hover",
          color: "text.disabled",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        {icon}
      </Box>
      <Typography variant="subtitle1" fontWeight={600}>
        {title}
      </Typography>
      <Typography variant="caption" color="text.secondary">
        {caption}
      </Typography>
    </Box>
  );
}

// One card per action plan (STANDARD and/or MANAGEMENT). Step completion and
// the final "Complete Action Plan" button are shown only to the plan's own
// action_owner_id, uniformly for both plan types. That ownership IS the
// authorisation — there is no privilege to check, which is why this takes no
// `can`: see canComplete below.
function ActionPlanCard({
  plan,
  currentUserId,
  disabled,
  riskStatus,
  userNames,
  onCompleteStep,
  onCompletePlan,
}: {
  plan: ActionPlanWithSteps;
  currentUserId: number | null;
  disabled: boolean;
  riskStatus: string;
  userNames: Map<number, string>;
  onCompleteStep: (planId: number, stepId: number) => void;
  onCompletePlan: (planId: number) => void;
}): JSX.Element {
  const isOwner = plan.action_owner_id !== null && plan.action_owner_id === currentUserId;
  // Mirrors the backend gate: steps/plan can only be completed while the
  // risk is actively being remediated, not before compliance approval.
  const riskActive = riskStatus === "IN_REMEDIATION" || riskStatus === "ESCALATED";
  // Being the plan's action owner IS the authorisation — no privilege check.
  // RISK_COMPLETE_ACTION_STEPS was retired along with the action-owner role,
  // because an Action Owner may be any employee and hold no role at all; the
  // server now authorises this purely on action_owner_id. Gating on a privilege
  // nobody can hold would hide the button from the only person who may use it.
  const canComplete = isOwner && riskActive;
  const allStepsDone = plan.steps.length > 0 && plan.steps.every((s) => s.status === "COMPLETED");
  const isManagement = plan.plan_type === "MANAGEMENT";
  const ownerName = plan.action_owner_id !== null ? (userNames.get(plan.action_owner_id) ?? null) : null;

  return (
    <SectionCard
      icon={isManagement ? <Briefcase size={16} /> : <Wrench size={16} />}
      iconBg={isManagement ? "#fff7ed" : "#eff6ff"}
      iconColor={isManagement ? "#b45309" : "#2563eb"}
      title={isManagement ? "Management Plan" : "Standard Plan"}
      headerExtra={<Chip label={plan.status} size="small" variant="outlined" />}
    >
      <Stack gap={1.5}>
        {plan.description && <Typography variant="body2">{plan.description}</Typography>}
        <Typography variant="caption" color="text.secondary">
          Action Owner: <strong>{ownerName ?? "Unassigned"}</strong>
        </Typography>
        {plan.steps.length > 0 && (
          <Stack gap={0.75}>
            {plan.steps.map((step) => (
              <Stack key={step.id} direction="row" gap={1} alignItems="center">
                <Typography variant="body2" color="text.secondary" fontWeight={600} sx={{ minWidth: 24 }}>
                  {step.step_no}.
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    flex: 1,
                    color: step.status === "COMPLETED" ? "text.secondary" : "text.primary",
                  }}
                >
                  {step.description}
                </Typography>
                {step.status === "COMPLETED" ? (
                  <Chip label="Done" size="small" color="success" variant="outlined" />
                ) : canComplete ? (
                  <Button
                    size="small"
                    variant="outlined"
                    disabled={disabled}
                    startIcon={<Check size={14} />}
                    onClick={() => onCompleteStep(plan.id, step.id)}
                  >
                    Mark Done
                  </Button>
                ) : null}
              </Stack>
            ))}
          </Stack>
        )}
        {canComplete && allStepsDone && plan.status !== "COMPLETED" && (
          <Button variant="contained" size="small" fullWidth disabled={disabled} onClick={() => onCompletePlan(plan.id)}>
            Complete Action Plan
          </Button>
        )}
      </Stack>
    </SectionCard>
  );
}

function ActionFooter({
  status,
  actions,
  disabled,
  can,
  actionPlans,
  isOverdue,
  isRiskOwner,
  isRiskAssigner,
  isManagementApprover,
  hasOpenEscalation,
}: {
  status: string;
  actions: DrawerActions;
  disabled: boolean;
  can: (privilege: string) => boolean;
  actionPlans: ActionPlanWithSteps[];
  isOverdue: boolean;
  // Per-risk identity — each already folds in the compliance-admin override,
  // so a compliance admin sees every action. See the backend's
  // requireRiskActor, which enforces the same rule server-side.
  isRiskOwner: boolean;
  isRiskAssigner: boolean;
  isManagementApprover: boolean;
  hasOpenEscalation: boolean;
}): JSX.Element | null {
  // isActor gates the pair on the named individual for that stage; compliance
  // approval has no named individual, so its callers pass true.
  const rejectAndApprove = (
    approveLabel: string,
    onApprove: () => void,
    approvePriv: string,
    rejectPriv: string,
    isActor: boolean,
  ) => {
    const showReject = can(rejectPriv) && isActor;
    const showApprove = can(approvePriv) && isActor;
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
      const showEdit = can(RiskPrivilege.UpdateRisk) && isRiskAssigner;
      const showCancel = can(RiskPrivilege.CancelRisk) && isRiskAssigner;
      const showReject = can(RiskPrivilege.OwnerRejectRisk) && isRiskOwner;
      const showOwnerApprove = can(RiskPrivilege.OwnerApproveRisk) && isRiskOwner;
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
      return rejectAndApprove("Approve as Risk Owner", actions.onOwnerApprove, RiskPrivilege.OwnerApproveRisk, RiskPrivilege.OwnerRejectRisk, isRiskOwner);

    case "PENDING_MANAGEMENT_APPROVAL":
      return rejectAndApprove("Approve as Management", actions.onManagementApprove, RiskPrivilege.ManagementApproveRisk, RiskPrivilege.ManagementRejectRisk, isManagementApprover);

    // Compliance approval is role-wide: any compliance admin may act, so there
    // is no named individual to check against.
    case "PENDING_COMPLIANCE_REVIEW":
      return rejectAndApprove("Approve (Compliance)", actions.onApprove, RiskPrivilege.ComplianceApproveRisk, RiskPrivilege.ComplianceRejectRisk, true);

    case "PENDING_OWNER_COMPLETION_APPROVAL":
      return rejectAndApprove("Approve Completion", actions.onOwnerApprove, RiskPrivilege.OwnerApproveRisk, RiskPrivilege.OwnerRejectRisk, isRiskOwner);

    // Closure-path management sign-off — reuses the same management approve
    // endpoint, which routes on the risk's current status.
    case "PENDING_MANAGEMENT_CLOSURE_APPROVAL":
      return rejectAndApprove("Approve Closure", actions.onManagementApprove, RiskPrivilege.ManagementApproveRisk, RiskPrivilege.ManagementRejectRisk, isManagementApprover);

    case "IN_REMEDIATION": {
      const showEdit = can(RiskPrivilege.UpdateRisk) && isRiskAssigner;
      // Reassessment is deliberately NOT identity-gated — the backend applies
      // no assigner check to it either, and keeping the two in step matters
      // more than tightening one side unilaterally.
      const showAssess = can(RiskPrivilege.AssessRisk);
      // At least one action plan must be COMPLETED first — not necessarily
      // all of them, since an abandoned STANDARD plan from a prior
      // escalation cycle shouldn't permanently block resubmission.
      const showComplete =
        can(RiskPrivilege.CompleteRisk) && isRiskAssigner && actionPlans.some((p) => p.status === "COMPLETED");
      // Additional plans are the assigner's call — typically after an
      // escalation review asked for more work.
      const showAddPlan = can(RiskPrivilege.ManageActionPlans) && isRiskAssigner;
      // Escalation happens automatically within 24h either way (the daily
      // job) — this just lets Compliance/Admin jump the queue for a risk
      // they've already spotted is overdue.
      const showEscalate = isOverdue && can(RiskPrivilege.EscalateRisk);
      if (!showEdit && !showAssess && !showComplete && !showEscalate && !showAddPlan) return null;
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
          {showEscalate && (
            <Button
              variant="outlined"
              color="error"
              fullWidth
              disabled={disabled}
              onClick={actions.onEscalate}
              sx={{ mb: showComplete ? 1 : 0 }}
            >
              Escalate
            </Button>
          )}
          {showAddPlan && (
            <Button
              variant="outlined"
              fullWidth
              disabled={disabled}
              onClick={actions.onAddActionPlan}
              sx={{ mb: showComplete ? 1 : 0 }}
            >
              Add Action Plan
            </Button>
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
      const showEdit = can(RiskPrivilege.UpdateRisk) && isRiskAssigner;
      const showResubmit = can(RiskPrivilege.SubmitRisk) && isRiskAssigner;
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

    case "ESCALATED": {
      // An escalation is answered with a comment, which returns the risk to its
      // assigner. Who may do that is decided server-side from the risk's level
      // (Management Approver for HIGH, a line manager for MEDIUM/LOW) — and a
      // lead holds no risk privilege by virtue of managing someone, so there is
      // nothing meaningful to gate on here. The button is offered to anyone who
      // can see the risk, and the server refuses if they aren't entitled.
      if (!hasOpenEscalation) return null;
      return (
        <Box sx={{ pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
          <Button variant="contained" fullWidth disabled={disabled} onClick={actions.onCommentEscalation}>
            Review Escalation
          </Button>
        </Box>
      );
    }

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
  onClose,
  actionPlans,
  actionPlansError,
  escalations,
  history,
  historyError,
  currentUserId,
  userNames,
  onCompleteStep,
  onCompletePlan,
  ...actions
}: RiskDetailDrawerProps): JSX.Element {
  // Every action in this drawer is gated on what the caller may do ON THIS
  // RISK, not on what they may do somewhere. A user can hold different roles in
  // different registers — Risk Owner in one, read-only in another — so the
  // session-wide set (canAnywhere, from GET /me/privileges) is the union across
  // all of them and would render an Approve button on risks the server refuses.
  //
  // effective_privileges comes back on the risk itself, already resolved in its
  // source register by the same rule the server enforces. Deriving it here from
  // a grant list instead would put a second copy of the access rule in the
  // browser, which is how the two drift apart.
  //
  // The session-wide set is deliberately NOT accepted as a prop: it is the
  // right answer for nav and route gating, and the wrong one for anything on a
  // specific risk.
  const can = useMemo(() => {
    const granted = new Set(detail?.effective_privileges ?? []);
    return (p: string): boolean => granted.has(p);
  }, [detail]);

  const status = detail?.workflow_status ?? "";
  const statusCfg = STATUS_CONFIG[status] ?? { label: status, color: "default" as const };
  const isOverdue = !!detail && calcDue(detail.implementation_date).daysLeft < 0;

  // Per-risk identity, mirroring the backend's requireRiskActor gate: holding
  // the privilege only says the caller may act on *some* risk, not that they're
  // the person *this* risk named. Without these the buttons would render and
  // then 403 on click.
  //
  // ComplianceApproveRisk is the same compliance-admin override the backend
  // uses (canOverrideAssignee) — it must stay in step with it, or the UI will
  // hide actions the server would have allowed.
  // An unresolved escalation is what puts the risk in the Overdue tab and
  // enables the review action — the workflow status doesn't say, because a
  // commented escalation is back to IN_REMEDIATION while still open.
  const openEscalation = escalations.find((e) => e.status === "OPEN") ?? null;

  const canOverrideAssignee = can(RiskPrivilege.ComplianceApproveRisk);
  const isRiskOwner = canOverrideAssignee || (!!detail && detail.owner_id === currentUserId);
  const isRiskAssigner = canOverrideAssignee || (!!detail && detail.assigner_id === currentUserId);
  const isManagementApprover =
    canOverrideAssignee || (!!detail && detail.management_approver_id === currentUserId);

  const [tab, setTab] = useState(0);
  // Reset to the first tab whenever a different risk is opened, so the
  // drawer doesn't retain the previous risk's active tab.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTab(0);
  }, [detail?.id]);

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
          ...dialogPaperSx,
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
                  {detail.workflow_status === "CLOSED" ? (
                    <Typography variant="caption" fontWeight={700} sx={{ color: "text.secondary", alignSelf: "center" }}>
                      —
                    </Typography>
                  ) : (
                    (() => {
                      const due = calcDue(detail.implementation_date);
                      return (
                        <Typography variant="caption" fontWeight={700} sx={{ color: due.color, alignSelf: "center" }}>
                          {due.label}
                        </Typography>
                      );
                    })()
                  )}
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

      {detail && !loading && !error && (
        <Tabs
          value={tab}
          onChange={(_, v: number) => setTab(v)}
          sx={{ px: 2, borderBottom: 1, borderColor: "divider", flexShrink: 0, minHeight: 44 }}
        >
          <Tab icon={<FileText size={15} />} iconPosition="start" label="Basic Information" sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }} />
          <Tab icon={<Shield size={15} />} iconPosition="start" label="Risk Treatment" sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }} />
          <Tab icon={<ListChecks size={15} />} iconPosition="start" label="Action Plans" sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }} />
          <Tab icon={<TrendingUp size={15} />} iconPosition="start" label="History" sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }} />
        </Tabs>
      )}

      {/* Scrollable content */}
      <Box sx={{ flex: 1, overflowY: "auto", px: 3, py: 2.5 }}>
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
            {openEscalation && (
              <Alert severity="warning" sx={{ mb: 2 }}>
                <Typography variant="caption" fontWeight={700} display="block">
                  Escalated on {formatDate(openEscalation.created_at)} — remediation passed its
                  implementation date
                </Typography>
                {openEscalation.decision ? (
                  <>
                    <Typography variant="caption" fontWeight={700} display="block" sx={{ mt: 0.5 }}>
                      Review comment
                    </Typography>
                    {openEscalation.decision}
                  </>
                ) : (
                  "Awaiting a review comment before this risk returns to its assigner."
                )}
              </Alert>
            )}

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

            <TabPanel value={tab} index={0}>
              <SectionCard icon={<FileText size={16} />} iconBg="#f1f5f9" iconColor="#475569" title="Identification">
                <Stack gap={1}>
                  <InfoGrid>
                    <InfoTile label="Source Register">{detail.source_register_name}</InfoTile>
                    <InfoTile label="Risk Identified Date">{formatDate(detail.risk_identified_date)}</InfoTile>
                    <InfoTile label="Identified By">{detail.identified_by_name ?? detail.identified_by_type ?? "—"}</InfoTile>
                  </InfoGrid>
                  {detail.risk_description && (
                    <InfoTile label="Description">{detail.risk_description}</InfoTile>
                  )}
                  {detail.impact_description && (
                    <InfoTile label="Impact Description">{detail.impact_description}</InfoTile>
                  )}
                </Stack>
              </SectionCard>

              <SectionCard icon={<Users size={16} />} iconBg="#eff6ff" iconColor="#2563eb" title="Ownership">
                <InfoGrid>
                  <InfoTile label="Assigned To">{detail.assigner_name}</InfoTile>
                  <InfoTile label="Risk Owner">{detail.owner_name}</InfoTile>
                  <InfoTile label="Management Approver">{detail.management_approver_name || "—"}</InfoTile>
                </InfoGrid>
              </SectionCard>

              <SectionCard icon={<Tag size={16} />} iconBg="#fef2f2" iconColor="#dc2626" title="Risk Category">
                {detail.risk_categories.length > 0 ? (
                  <Stack direction="row" flexWrap="wrap" gap={0.75}>
                    {detail.risk_categories.map((cat) => (
                      <Chip key={cat.id} label={cat.name} size="small" variant="outlined" />
                    ))}
                  </Stack>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    No risk category assigned.
                  </Typography>
                )}
              </SectionCard>

              <SectionCard icon={<LinkIcon size={16} />} iconBg="#f5f3ff" iconColor="#7c3aed" title="Compliance References">
                {detail.compliance_references.length > 0 ? (
                  <Stack direction="row" flexWrap="wrap" gap={0.75}>
                    {detail.compliance_references.map((ref) => (
                      <Chip key={ref.id} label={ref.name} size="small" variant="outlined" />
                    ))}
                  </Stack>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    No compliance references linked.
                  </Typography>
                )}
              </SectionCard>
            </TabPanel>

            <TabPanel value={tab} index={1}>
              <SectionCard icon={<Shield size={16} />} iconBg="#fff7ed" iconColor="#b45309" title="Treatment">
                <InfoGrid>
                  <InfoTile label="Assignment Team">{detail.assignment_team_name}</InfoTile>
                  <InfoTile label="Treatment Strategy">{detail.treatment_strategy}</InfoTile>
                </InfoGrid>
              </SectionCard>

              <SectionCard icon={<Calendar size={16} />} iconBg="#dcfce7" iconColor="#16a34a" title="Timeline & Progress">
                <Stack gap={1}>
                  <InfoGrid>
                    <InfoTile label="Implementation Date">{formatDate(detail.implementation_date)}</InfoTile>
                    <InfoTile label="Reassessment Date">{formatDate(detail.reassessment_date)}</InfoTile>
                  </InfoGrid>
                  {detail.progress && <InfoTile label="Progress">{detail.progress}</InfoTile>}
                </Stack>
              </SectionCard>

              <SectionCard icon={<MessageSquare size={16} />} iconBg="#fff7ed" iconColor="#ea580c" title="References">
                <Stack gap={1}>
                  <InfoGrid>
                    <InfoTile label="Email Subject">{detail.email_subject}</InfoTile>
                    <InfoTile label="Git Issue URL">
                      {detail.git_issue_url ? (
                        <a href={detail.git_issue_url} target="_blank" rel="noreferrer">
                          {detail.git_issue_url}
                        </a>
                      ) : (
                        "—"
                      )}
                    </InfoTile>
                  </InfoGrid>
                  {detail.remarks && <InfoTile label="Remarks">{detail.remarks}</InfoTile>}
                </Stack>
              </SectionCard>
            </TabPanel>

            <TabPanel value={tab} index={2}>
              {actionPlansError ? (
                <Alert severity="error">{actionPlansError}</Alert>
              ) : actionPlans.length > 0 ? (
                actionPlans.map((plan) => (
                  <ActionPlanCard
                    key={plan.id}
                    plan={plan}
                    currentUserId={currentUserId}
                    disabled={actionsDisabled}
                    riskStatus={status}
                    userNames={userNames}
                    onCompleteStep={onCompleteStep}
                    onCompletePlan={onCompletePlan}
                  />
                ))
              ) : (
                <EmptyState
                  icon={<ListChecks size={28} />}
                  title="No action plans yet"
                  caption="An action plan will appear here once one is created for this risk."
                />
              )}
            </TabPanel>

            <TabPanel value={tab} index={3}>
              {historyError ? (
                <Alert severity="error">{historyError}</Alert>
              ) : history.length > 0 ? (
                <SectionCard icon={<TrendingUp size={16} />} iconBg="#f1f5f9" iconColor="#475569" title="History">
                  <RiskHistoryTimeline entries={history} />
                </SectionCard>
              ) : (
                <EmptyState
                  icon={<TrendingUp size={28} />}
                  title="No history yet"
                  caption="Actions taken on this risk will appear here."
                />
              )}
            </TabPanel>
          </>
        ) : null}
      </Box>

      {/* Fixed action footer */}
      {detail && !loading && !error && (
        <Box sx={{ px: 3, pb: 3, pt: 0 }}>
          <ActionFooter
            status={status}
            actions={actions}
            disabled={actionsDisabled}
            can={can}
            actionPlans={actionPlans}
            isOverdue={isOverdue}
            isRiskOwner={isRiskOwner}
            isRiskAssigner={isRiskAssigner}
            isManagementApprover={isManagementApprover}
            hasOpenEscalation={!!openEscalation}
          />
        </Box>
      )}
    </Drawer>
  );
}

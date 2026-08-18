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
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Drawer,
  IconButton,
  Paper,
  Skeleton,
  Stack,
  Step,
  StepLabel,
  Stepper,
  Tab,
  Tabs,
  TextField,
} from "@wso2/oxygen-ui";
import { Box, Typography } from "@wso2/oxygen-ui";
import {
  AlertCircle,
  CalendarDays,
  CheckCircle2,
  Clock,
  ClipboardCheck,
  FileText,
  FileUp,
  History,
  MessageSquare,
  RotateCcw,
  Trash2,
  Upload,
  Users,
  X,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { useEffect, useRef, useState, type JSX } from "react";
import ControlStatusChip from "@modules/audit/components/ControlStatusChip";
import UserAvatar from "@modules/audit/components/UserAvatar";
import { formatAuditDate } from "@modules/audit/utils/format";
import EvidenceUploadBox from "@modules/audit/components/EvidenceUploadBox";
import SubmittedEvidenceList from "@modules/audit/components/SubmittedEvidenceList";
import ControlHistoryTimeline from "@modules/audit/components/ControlHistoryTimeline";
import CommentsSection from "@modules/audit/components/CommentsSection";
import AIValidationCard from "@modules/audit/components/AIValidationCard";
import PopulationFileList from "@modules/audit/components/PopulationFileList";
import { useGetPopulation } from "@modules/audit/api/useGetPopulation";
import { useDeletePopulationAttestation } from "@modules/audit/api/useDeletePopulationAttestation";
import { usePopulationReview } from "@modules/audit/api/usePopulationReview";
import { usePopulationValidate } from "@modules/audit/api/usePopulationValidate";
import { useSubmitSample } from "@modules/audit/api/useSubmitSample";
import { useRequestSampleTime } from "@modules/audit/api/useRequestSampleTime";
import { useValidateEvidence } from "@modules/audit/api/useValidateEvidence";
import { useReviewEvidence } from "@modules/audit/api/useReviewEvidence";
import { useCurrentUserEmail } from "@modules/audit/hooks/useCurrentUserEmail";
import { useOverrideControlStatus } from "@modules/audit/api/useOverrideControlStatus";
import { isAssignedAuditor } from "@modules/audit/utils/auditor";
import type { AuditControl, ControlStatus } from "@modules/audit/types/audit";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import { CONTROL_STATUS_LABELS } from "@modules/audit/utils/controlStatus";

interface ControlDrawerProps {
  control: AuditControl | null;
  open: boolean;
  onClose: () => void;
}

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

// ─── Info tile ────────────────────────────────────────────────────────────────
// Fixed grid cell, not a flex-wrap row: a returning user learns "Due Date is
// top-right, Owner is bottom-left" and that only holds if every field keeps
// the same slot every time, regardless of content length or drawer width.

function InfoTile({
  label,
  children,
  accent = false,
}: {
  label: string;
  children: React.ReactNode;
  accent?: boolean;
}): JSX.Element {
  return (
    <Box
      sx={{
        px: 1.25,
        py: 1,
        borderRadius: 1,
        border: "1px solid",
        borderColor: accent ? "#dc2626" : "divider",
        bgcolor: "action.hover",
        display: "flex",
        flexDirection: "column",
        gap: 0.4,
      }}
    >
      <Typography
        variant="caption"
        // Matches the header's "Overdue" chip (#dc2626) exactly, rather than
        // the theme's error.main token which can render a different shade.
        sx={{ fontWeight: 500, fontSize: "0.67rem", lineHeight: 1, color: accent ? "#dc2626" : "text.secondary" }}
      >
        {label}
      </Typography>
      {children}
    </Box>
  );
}

// Due Date is the one field worth catching at a glance, so it always renders
// through this tile (red-accented when overdue) rather than a plain value —
// but it still sits in the same grid cell as every other tile.
function DueDateTile({
  label,
  date,
  overdue = false,
}: {
  label: string;
  date: string | null;
  overdue?: boolean;
}): JSX.Element {
  return (
    <InfoTile label={label} accent={overdue}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
        <CalendarDays size={13} color={overdue ? "#dc2626" : "#64748b"} style={{ flexShrink: 0 }} />
        <Typography variant="body2" fontWeight={700} fontSize="0.8rem" sx={{ color: overdue ? "#dc2626" : "text.primary" }}>
          {date ? formatAuditDate(date) : "—"}
        </Typography>
      </Box>
    </InfoTile>
  );
}

// ─── Section card ─────────────────────────────────────────────────────────────

interface SectionCardProps {
  icon: React.ReactNode;
  iconColor?: string;
  iconBg?: string;
  title: string;
  children: React.ReactNode;
  noPad?: boolean;
  flexContent?: boolean;
}

function SectionCard({
  icon,
  // A theme token, not a fixed hex: "#475569" reads fine in light mode but is
  // nearly black against a dark background, so it must resolve per-theme.
  iconColor = "text.secondary",
  iconBg = "#f1f5f9",
  title,
  children,
  noPad = false,
  flexContent = false,
}: SectionCardProps): JSX.Element {
  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: "hidden", display: "flex", flexDirection: "column" }}>
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
        <Typography variant="subtitle2" fontWeight={700}>
          {title}
        </Typography>
      </Box>
      <Box sx={noPad ? undefined : { p: 2.5, ...(flexContent && { display: "flex", flexDirection: "column", flex: 1 }) }}>{children}</Box>
    </Paper>
  );
}

// ─── Tab panel ────────────────────────────────────────────────────────────────

function TabPanel({ value, index, children }: { value: number; index: number; children: React.ReactNode }): JSX.Element {
  return (
    <Box
      role="tabpanel"
      hidden={value !== index}
      sx={{ flex: 1, overflowY: "auto", display: value === index ? "flex" : "none", flexDirection: "column" }}
    >
      {value === index && (
        <Box sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 2.5 }}>
          {children}
        </Box>
      )}
    </Box>
  );
}

// ─── OE evidence section ──────────────────────────────────────────────────────

const OE_STEPS = ["Submit Population", "Sample Selection", "Submit Evidence", "Review"] as const;

function oeActiveStep(status: ControlStatus): number {
  if (
    status === "POPULATION_PENDING" ||
    status === "POPULATION_INTERNAL_REVIEW" ||
    status === "POPULATION_UNDER_VALIDATION" ||
    status === "POPULATION_NEED_CLARIFICATION"
  ) return 0;
  if (
    status === "POPULATION_COMPLETE" ||
    status === "AWAITING_SAMPLE" ||
    status === "SUBMITTED_SAMPLE"
  ) return 1;
  // EVIDENCE_NEED_CLARIFICATION (auditor rejected at validation) buckets with
  // EVIDENCE_PENDING (internal reviewer rejected) — both are "team must
  // resubmit" and render identically below (driven by control.comments, not
  // the exact status), the same way designActiveStep treats its own
  // equivalent statuses identically. Without this, EVIDENCE_NEED_CLARIFICATION
  // fell into the step-3 bucket below with no matching status check, so
  // nothing rendered in the Evidence tab after an auditor rejection.
  if (status === "EVIDENCE_PENDING" || status === "EVIDENCE_NEED_CLARIFICATION") return 2;
  if (status === "COMPLETE") return 4;
  return 3; // EVIDENCE_INTERNAL_REVIEW, EVIDENCE_UNDER_VALIDATION
}

// Statuses where the OE control is still in the population phase — the
// Overview tab shows the "Population Requirement" text for these, and
// switches to "Evidence Requirement" for every status after (sample
// submitted onward), since the team's job has shifted from describing the
// population to providing evidence.
const OE_POPULATION_PHASE_STATUSES = new Set<ControlStatus>([
  "POPULATION_PENDING",
  "POPULATION_INTERNAL_REVIEW",
  "POPULATION_UNDER_VALIDATION",
  "POPULATION_NEED_CLARIFICATION",
  "POPULATION_COMPLETE",
  "AWAITING_SAMPLE",
]);

// Statuses that render their own review/validate decision card — the
// population AI Validation placeholder gets its own placement right before
// that decision for these two (between submission and the decision), instead
// of the general "after everything" placement every other population
// sub-state uses.
const POPULATION_REVIEW_STATUSES = new Set<ControlStatus>([
  "POPULATION_INTERNAL_REVIEW",
  "POPULATION_UNDER_VALIDATION",
]);

// ─── Design evidence section ──────────────────────────────────────────────────

const DESIGN_STEPS = ["Evidence Pending", "Internal Review", "Under Validation", "Complete"] as const;

function designActiveStep(status: ControlStatus): number {
  if (status === "EVIDENCE_INTERNAL_REVIEW") return 1;
  if (status === "EVIDENCE_UNDER_VALIDATION") return 2;
  if (status === "COMPLETE") return 3;
  return 0;
}

function DesignEvidenceSection({
  control,
  onStatusChange,
  canSubmitEvidence,
  canManageControls,
}: {
  control: AuditControl;
  onStatusChange: (s: ControlStatus) => void;
  canSubmitEvidence: boolean;
  canManageControls: boolean;
}): JSX.Element {
  const activeStep = designActiveStep(control.status);

  return (
    <>
      <Paper variant="outlined" sx={{ borderRadius: 2, p: { xs: 1.5, sm: 2 }, overflow: "hidden" }}>
        <Stepper activeStep={activeStep} alternativeLabel sx={{ "& .MuiStepLabel-label": { fontSize: "0.72rem", mt: 0.5 } }}>
          {DESIGN_STEPS.map((label) => (
            <Step key={label}><StepLabel>{label}</StepLabel></Step>
          ))}
        </Stepper>
      </Paper>

      {activeStep === 0 && (
        <SectionCard icon={<FileUp size={16} />} iconBg="transparent" title="Evidence Submission" flexContent>
          {/* A round left over from before the status reverted here (a
              reject, or an admin override) is still on record — show it
              with delete so the team can clear it and resubmit, instead
              of it staying invisible until a fresh upload creates a new
              round (which used to leave the old one an orphaned duplicate).
              rejectionReason only when plain EVIDENCE_PENDING — this activeStep
              also covers EVIDENCE_NEED_CLARIFICATION, whose own status label
              already conveys "sent back", unlike bare EVIDENCE_PENDING. */}
          <SubmittedEvidenceList
            auditId={control.auditId}
            controlId={control.id}
            canDelete={canSubmitEvidence || canManageControls}
            rejectionReason={control.status === "EVIDENCE_PENDING" ? (control.comments ?? null) : undefined}
            onStatusChange={(s) => onStatusChange(s as ControlStatus)}
          />
          {canSubmitEvidence && (
            <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
              <EvidenceUploadBox
                auditId={control.auditId}
                controlId={control.id}
                hint="PDF, XLSX, PNG up to 25 MB each"
                buttonLabel="Submit Evidence"
                onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
              />
            </Box>
          )}
        </SectionCard>
      )}

      {/* Submitted files + add-more: same card position as upload so layout stays
          stable. Removing a file and adding another together cover editing a
          submission, so there is no separate withdraw step. */}
      {activeStep === 1 && (
        <SectionCard icon={<FileUp size={16} />} iconBg="transparent" title="Evidence Submission">
          <SubmittedEvidenceList
            auditId={control.auditId}
            controlId={control.id}
            canDelete={canSubmitEvidence || canManageControls}
            onStatusChange={(s) => onStatusChange(s as ControlStatus)}
          />
          {canSubmitEvidence && (
            <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
                <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#b45309", flexShrink: 0 }} />
                <Typography variant="body2" color="text.secondary">
                  Under internal review - you can still add or remove files.
                </Typography>
              </Box>
              <EvidenceUploadBox
                auditId={control.auditId}
                controlId={control.id}
                hint="PDF, XLSX, PNG up to 25 MB each"
                buttonLabel="Add Files"
                evidenceMode="append"
                onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
              />
            </Box>
          )}
        </SectionCard>
      )}

      {activeStep === 2 && (
        <SectionCard icon={<ClipboardCheck size={16} />} iconBg="transparent" title="Submitted Evidence">
          {/* Locked for the team once the round reaches auditor validation —
              canDelete is ManageControls-only here, for the same admin
              cleanup case as a status override landing the control back on
              this step. */}
          <SubmittedEvidenceList auditId={control.auditId} controlId={control.id} canDelete={canManageControls} />
          <Box sx={{ mt: 1.5, py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "action.hover", display: "flex", alignItems: "center", gap: 1 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#7c3aed", flexShrink: 0 }} />
            <Typography variant="body2" color="text.secondary">Passed internal review. External auditor is validating.</Typography>
          </Box>
        </SectionCard>
      )}

      {activeStep === 3 && (
        <SectionCard icon={<CheckCircle2 size={16} />} iconBg="transparent" title="Submitted Evidence">
          <SubmittedEvidenceList auditId={control.auditId} controlId={control.id} />
          <Box sx={{ mt: 1.5, py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "rgba(22,163,74,0.06)", display: "flex", alignItems: "center", gap: 1 }}>
            <CheckCircle2 size={14} color="#16a34a" />
            <Typography variant="body2" color="text.secondary">
              {control.comments ?? "All evidence reviewed and approved."}
            </Typography>
          </Box>
        </SectionCard>
      )}

      {/* AI validation — always below whichever submission/submitted-evidence
          card is active for the current step, so it never ends up above it
          regardless of how far the control has progressed. */}
      {canSubmitEvidence && (
        <AIValidationCard auditId={control.auditId} controlId={control.id} variant="submitter" />
      )}
    </>
  );
}

// ─── OE evidence section ──────────────────────────────────────────────────────

// AttestationNote renders a population round's written note (a fileless
// submit, or a note alongside files) with an optional remove button — shared
// by SubmittedPopulationFiles and PopulationSubmissionCard so the delete
// wiring (useDeletePopulationAttestation) exists in one place instead of two.
// Blanking the note never touches the round's files or status (see
// deletePopulationAttestation on the backend).
function AttestationNote({
  auditId,
  controlId,
  attestation,
  filesEmpty,
  canDelete,
}: {
  auditId: number;
  controlId: number;
  attestation: string;
  filesEmpty: boolean;
  canDelete: boolean;
}): JSX.Element {
  const deleteAttestation = useDeletePopulationAttestation();
  return (
    <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1, px: 1.25, py: 0.85, borderRadius: 1, border: "1px solid", borderColor: "divider", bgcolor: "action.hover" }}>
      <FileText size={15} style={{ flexShrink: 0, marginTop: 2 }} />
      <Box sx={{ flex: 1 }}>
        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: "block" }}>
          {filesEmpty ? "Completed without files." : "Note"}
        </Typography>
        <Typography variant="body2" sx={{ lineHeight: 1.6 }}>{attestation}</Typography>
        {deleteAttestation.isError && (
          <Typography variant="caption" color="error" sx={{ display: "block", mt: 0.5 }}>
            {(deleteAttestation.error as Error).message}
          </Typography>
        )}
      </Box>
      {canDelete && (
        <IconButton
          size="small"
          aria-label="Remove note"
          disabled={deleteAttestation.isPending}
          onClick={() => deleteAttestation.mutate({ auditId, controlId })}
          sx={{ p: 0.5, color: "error.main", "&:hover": { bgcolor: "rgba(220,38,38,0.06)" } }}
        >
          {deleteAttestation.isPending ? <CircularProgress size={13} color="inherit" /> : <Trash2 size={14} />}
        </IconButton>
      )}
    </Box>
  );
}

// SubmittedPopulationFiles renders the round's already-recorded POPULATION-kind
// files (with a remove button) inline — no card of its own — so it lives
// inside the same Submit/Resubmit Population card as the upload box, the same
// way SubmittedEvidenceList sits inside DesignEvidenceSection's Evidence
// Submission card. Always renders something (even "no files yet") instead of
// disappearing, so the resubmit card doesn't jump around depending on whether
// a round has files on record.
//
// Pass `rejectionReason` only when control.status is plain POPULATION_PENDING
// — same contract and reasoning as SubmittedEvidenceList's prop of the same
// name: internal-review reject and a status override both land on plain
// PENDING (indistinguishable from a first-time submission), while
// POPULATION_NEED_CLARIFICATION's own label already says "sent back", so
// never pass this prop there.
function SubmittedPopulationFiles({
  auditId,
  controlId,
  rejectionReason,
}: {
  auditId: number;
  controlId: number;
  rejectionReason?: string | null;
}): JSX.Element {
  const population = useGetPopulation(auditId, controlId, true);
  const files = population.data?.populationFiles ?? [];
  const attestation = population.data?.round.attestation ?? null;

  if (population.isLoading) {
    return <Skeleton variant="rounded" height={44} />;
  }

  const showResubmissionNote = rejectionReason !== undefined && (files.length > 0 || Boolean(rejectionReason));
  const resubmissionNote = showResubmissionNote && (
    <Box sx={{ display: "flex", alignItems: "flex-start", gap: 0.75 }}>
      <RotateCcw size={13} color="#b45309" style={{ flexShrink: 0, marginTop: 2 }} />
      <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.6 }}>
        {rejectionReason ? `Sent back for revision: ${rejectionReason}` : "Resubmit to continue."}
      </Typography>
    </Box>
  );
  // A round submitted with a note instead of (or alongside) files — same
  // "Completed without files" treatment as SubmittedEvidenceList's fileless
  // rounds, just for the one persistent population round instead of a list.
  // canDelete unconditional, same as this component's PopulationFileList call
  // below — this card is only reached from a team-editable status, and the
  // backend's own privilege+assignment gate is the actual enforcement.
  const attestationNote = attestation && (
    <AttestationNote
      auditId={auditId}
      controlId={controlId}
      attestation={attestation}
      filesEmpty={files.length === 0}
      canDelete
    />
  );

  if (files.length === 0) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
        {resubmissionNote}
        {attestationNote}
        {!attestation && (
          <Typography variant="body2" color="text.secondary">
            No population files on record yet.
          </Typography>
        )}
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      {resubmissionNote}
      {attestationNote}
      <PopulationFileList files={files} emptyText="" auditId={auditId} controlId={controlId} canDelete />
    </Box>
  );
}

function SampleSelectionCard({
  auditId,
  controlId,
  sampleReference,
}: {
  auditId: number;
  controlId: number;
  sampleReference: string | null;
}): JSX.Element {
  const population = useGetPopulation(auditId, controlId, true);
  const hasNote = Boolean(sampleReference);
  const sampleFiles = population.data?.sampleFiles ?? [];

  return (
    <SectionCard
      icon={<ClipboardCheck size={16} />}
      iconBg="transparent"
      title="Sample Selected by Auditor"
    >
      {!hasNote && sampleFiles.length === 0 && !population.isLoading && (
        <Typography variant="body2" color="text.secondary">
          Sample details will appear here once the auditor completes selection.
        </Typography>
      )}

      {/* Same "Completed without files" visual language as SubmittedEvidenceList's
          fileless rounds and the population attestation note (PopulationSubmissionCard/
          SubmittedPopulationFiles) — neutral action.hover/divider tokens, not a
          one-off accent color, so every "note standing in for files" surface
          in the drawer looks consistent. */}
      {hasNote && (
        <Box
          sx={{
            display: "flex",
            alignItems: "flex-start",
            gap: 1,
            px: 1.25,
            py: 0.85,
            borderRadius: 1,
            border: "1px solid",
            borderColor: "divider",
            bgcolor: "action.hover",
            mb: sampleFiles.length > 0 ? 1.5 : 0,
          }}
        >
          <FileText size={15} style={{ flexShrink: 0, marginTop: 2 }} />
          <Box sx={{ flex: 1 }}>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: "block" }}>
              Auditor Note
            </Typography>
            <Typography variant="body2" sx={{ lineHeight: 1.6 }}>{sampleReference}</Typography>
          </Box>
        </Box>
      )}

      {population.isLoading && <Skeleton variant="rounded" height={44} />}
      {sampleFiles.length > 0 && <PopulationFileList files={sampleFiles} emptyText="" />}
    </SectionCard>
  );
}

// PopulationReviewCard renders the internal-reviewer or auditor decision
// surface for a submitted population round: description + Approve/Reject
// only. The files themselves are not repeated here for either mode — they're
// already shown in the Population Submission card rendered just above this
// one (editable during review, read-only during validation), so showing them
// again would just duplicate that list.
function PopulationReviewCard({
  auditId,
  controlId,
  mode,
  onDecided,
}: {
  auditId: number;
  controlId: number;
  mode: "review" | "validate";
  onDecided: (status: ControlStatus) => void;
}): JSX.Element {
  const review = usePopulationReview();
  const validate = usePopulationValidate();
  const mutation = mode === "review" ? review : validate;

  const title = mode === "review" ? "Population Internal Review" : "Population Auditor Validation";
  const description = mode === "review"
    ? "Review the submitted population before it goes to the auditor."
    : "Validate the population before it moves to sample selection.";
  const color = mode === "review" ? "#b45309" : "#7c3aed";
  const hoverColor = mode === "review" ? "#92400e" : "#6d28d9";

  function decide(decision: "APPROVE" | "REJECT") {
    mutation.mutate(
      { auditId, controlId, decision },
      { onSuccess: (data) => onDecided(data.status as ControlStatus) },
    );
  }

  return (
    <SectionCard icon={<ClipboardCheck size={16} />} iconBg="transparent" title={title}>
      <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.7 }}>
        {description}
      </Typography>
      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
        <Button
          variant="contained"
          disableElevation
          disabled={mutation.isPending}
          startIcon={mutation.isPending ? <CircularProgress size={15} color="inherit" /> : <CheckCircle2 size={15} />}
          onClick={() => decide("APPROVE")}
          sx={{ textTransform: "none", fontWeight: 600, bgcolor: color, color: "#fff", "&:hover": { bgcolor: hoverColor } }}
        >
          Approve
        </Button>
        <Button
          variant="outlined"
          disabled={mutation.isPending}
          startIcon={<XCircle size={15} />}
          onClick={() => decide("REJECT")}
          sx={{ textTransform: "none", fontWeight: 600, color: "#dc2626", borderColor: "#dc2626", "&:hover": { borderColor: "#b91c1c", bgcolor: "rgba(220,38,38,0.04)" } }}
        >
          Reject
        </Button>
      </Box>
      {mutation.isError && (
        <Alert severity="error" sx={{ mt: 1, fontSize: "0.8rem" }}>{(mutation.error as Error).message}</Alert>
      )}
    </SectionCard>
  );
}

// SampleWaitingCard is what the team (and any non-auditor) sees while the
// auditor is choosing the sample — no action available, just a status message.
function SampleWaitingCard({ status }: { status: ControlStatus }): JSX.Element {
  const preparing = status === "AWAITING_SAMPLE";
  return (
    <SectionCard icon={<Clock size={16} />} iconBg="transparent" title="Auditor Selecting Sample">
      <Box sx={{ py: 2, display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5, textAlign: "center" }}>
        <Box sx={{ width: 52, height: 52, borderRadius: "50%", bgcolor: "#eff6ff", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <Clock size={24} color="#1d4ed8" />
        </Box>
        <Typography variant="body2" fontWeight={600}>Population approved</Typography>
        <Typography variant="caption" color="text.secondary" sx={{ maxWidth: 320, lineHeight: 1.65 }}>
          {preparing
            ? "The auditor is preparing the sample and will submit it shortly."
            : "The external auditor is selecting a sample for evidence collection."}
        </Typography>
      </Box>
    </SectionCard>
  );
}

// SampleUploadCard is the auditor's sample-selection form: files + a required
// note, plus an optional "Request More Time" escape hatch.
// With editMode it doubles as the post-submission editor (status SUBMITTED_SAMPLE
// only — the round locks once evidence review starts): it also lists the
// already-recorded sample files with a remove button and prefills the note.
function SampleUploadCard({
  auditId,
  controlId,
  canRequestMoreTime,
  onSubmitted,
  editMode = false,
  initialNote = "",
}: {
  auditId: number;
  controlId: number;
  canRequestMoreTime: boolean;
  onSubmitted: (status: ControlStatus) => void;
  editMode?: boolean;
  initialNote?: string;
}): JSX.Element {
  const [files, setFiles] = useState<File[]>([]);
  const [note, setNote] = useState(initialNote);
  const inputRef = useRef<HTMLInputElement>(null);
  const submitSample = useSubmitSample();
  const requestTime = useRequestSampleTime();
  // Always fetched (not just in editMode): an admin status override can land
  // the control back on POPULATION_COMPLETE/AWAITING_SAMPLE with sample files
  // already on the round (the override cascade demotes the round's status but
  // never deletes its files — see useOverrideControlStatus). Gating this on
  // editMode hid those files here until the auditor's next submit dragged
  // them back into view alongside the new ones, reading as files reappearing
  // out of nowhere.
  const population = useGetPopulation(auditId, controlId, true);
  const existingFiles = population.data?.sampleFiles ?? [];
  const busy = submitSample.isPending || requestTime.isPending;

  // initialNote comes from the parent's (possibly pre-refetch, stale) control
  // prop — right after the first submit it can still read the old value while
  // this card mounts in edit mode. Once useGetPopulation's own fetch lands, it
  // is the authoritative source, so sync note from it — but NOT just once: the
  // query cache can already hold a stale snapshot from an unrelated earlier
  // subscriber (e.g. PopulationSubmissionCard, which keeps the same query key
  // mounted elsewhere) at the exact moment this component mounts, so the
  // FIRST population.data this effect sees can itself be pre-refetch-stale —
  // syncing once and locking via a ref would sync to that stale value and then
  // ignore the real fresh data that lands a moment later from the background
  // refetch useSubmitSample's invalidation kicked off. Instead, keep re-syncing
  // on every population.data change until the user actually edits the field.
  const [noteDirty, setNoteDirty] = useState(false);
  useEffect(() => {
    if (editMode && population.data && !noteDirty) {
      const syncedNote = population.data.sampleReference ?? "";
      queueMicrotask(() => setNote(syncedNote));
    }
  }, [editMode, population.data, noteDirty]);

  function addFiles(list: FileList | null) {
    if (!list) return;
    const incoming = Array.from(list);
    setFiles((prev) => {
      const seen = new Set(prev.map((f) => f.name + f.size));
      return [...prev, ...incoming.filter((f) => !seen.has(f.name + f.size))];
    });
  }
  function removeFile(idx: number) {
    setFiles((prev) => prev.filter((_, i) => i !== idx));
  }
  function handleSubmit() {
    submitSample.mutate(
      { auditId, controlId, files, note },
      { onSuccess: () => { setFiles([]); if (!editMode) setNote(""); onSubmitted("SUBMITTED_SAMPLE"); } },
    );
  }
  function handleRequestTime() {
    requestTime.mutate({ auditId, controlId }, { onSuccess: () => onSubmitted("AWAITING_SAMPLE") });
  }

  return (
    <SectionCard icon={<ClipboardCheck size={16} />} iconBg="transparent" title={editMode ? "Update Sample" : "Select Sample"} flexContent>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5, lineHeight: 1.7 }}>
        {editMode
          ? "Add more sample files, remove ones no longer needed, or update the note below."
          : "Upload the sample file(s)"}
      </Typography>

      {existingFiles.length > 0 && (
        <Box sx={{ mb: 1.5 }}>
          <PopulationFileList files={existingFiles} emptyText="" auditId={auditId} controlId={controlId} canDelete />
        </Box>
      )}

      <input
        ref={inputRef}
        type="file"
        multiple
        hidden
        onChange={(e) => { addFiles(e.target.files); e.target.value = ""; }}
      />
      <Button
        variant="outlined"
        onClick={() => inputRef.current?.click()}
        disabled={busy}
        startIcon={<Upload size={15} />}
        sx={{ textTransform: "none", mb: 1.5, alignSelf: "flex-start" }}
      >
        Choose Files
      </Button>

      {files.length > 0 && (
        <Box sx={{ mb: 1.5, display: "flex", flexDirection: "column", gap: 0.5 }}>
          {files.map((f, i) => (
            <Box key={f.name + f.size + i} sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.25, py: 0.75, borderRadius: 1, bgcolor: "action.hover" }}>
              <FileUp size={14} />
              <Typography variant="caption" sx={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {f.name}
              </Typography>
              <IconButton size="small" aria-label={`Remove ${f.name}`} disabled={busy} onClick={() => removeFile(i)} sx={{ p: 0.25 }}>
                <X size={13} />
              </IconButton>
            </Box>
          ))}
        </Box>
      )}

      <TextField
        multiline
        minRows={2}
        placeholder="Sample note (e.g. which items to provide evidence for)"
        value={note}
        onChange={(e) => { setNote(e.target.value); setNoteDirty(true); }}
        disabled={busy}
        fullWidth
        size="small"
        sx={{ mb: 1.5 }}
      />

      {submitSample.isError && (
        <Alert severity="error" sx={{ mb: 1.5, fontSize: "0.8rem" }}>{(submitSample.error as Error).message}</Alert>
      )}
      {requestTime.isError && (
        <Alert severity="error" sx={{ mb: 1.5, fontSize: "0.8rem" }}>{(requestTime.error as Error).message}</Alert>
      )}

      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
        <Button
          variant="contained"
          disableElevation
          disabled={(files.length === 0 && note.trim() === "" && existingFiles.length === 0) || busy}
          startIcon={submitSample.isPending ? <CircularProgress size={15} color="inherit" /> : <FileUp size={15} />}
          onClick={handleSubmit}
          sx={{ textTransform: "none", fontWeight: 600 }}
        >
          {submitSample.isPending ? (editMode ? "Updating…" : "Submitting…") : (editMode ? "Update Sample" : "Submit Sample")}
        </Button>
        {canRequestMoreTime && (
          <Button
            variant="outlined"
            disabled={busy}
            startIcon={requestTime.isPending ? <CircularProgress size={13} color="inherit" /> : <Clock size={15} />}
            onClick={handleRequestTime}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            Request More Time
          </Button>
        )}
      </Box>
    </SectionCard>
  );
}

// PopulationSubmissionCard is the one persistent place population files are
// shown, from the first internal review all the way through Complete — it
// does not vanish once the auditor approves the population, matching how
// Design's evidence submission card stays visible for the rest of the
// control's life. Editable only while `editable` is true (internal review,
// before the round locks); read-only everywhere after.
function PopulationSubmissionCard({
  auditId,
  controlId,
  editable,
  canDelete,
  onStatusChange,
}: {
  auditId: number;
  controlId: number;
  editable: boolean;
  canDelete: boolean;
  onStatusChange: (s: ControlStatus) => void;
}): JSX.Element {
  const population = useGetPopulation(auditId, controlId, true);
  const attestation = population.data?.round.attestation ?? null;
  const files = population.data?.populationFiles ?? [];
  return (
    <SectionCard icon={<FileUp size={16} />} iconBg="transparent" title="Population Submission">
      {population.isLoading ? (
        <Skeleton variant="rounded" height={56} />
      ) : population.isError ? (
        // A failed fetch must read as an error, not silently fall through to
        // an empty files array — that would show "No population files
        // submitted yet" for a round that actually has files, which is
        // actively misleading rather than just unhelpful.
        <Alert severity="error" sx={{ fontSize: "0.8rem" }}>
          {(population.error as Error)?.message ?? "Failed to load the submitted population."}
        </Alert>
      ) : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
          {/* A round submitted with a note instead of (or alongside) files —
              same "Completed without files" treatment as
              SubmittedEvidenceList's fileless rounds. */}
          {attestation && (
            <AttestationNote
              auditId={auditId}
              controlId={controlId}
              attestation={attestation}
              filesEmpty={files.length === 0}
              canDelete={canDelete}
            />
          )}
          {(files.length > 0 || !attestation) && (
            <PopulationFileList
              files={files}
              emptyText="No population files submitted yet."
              auditId={auditId}
              controlId={controlId}
              canDelete={canDelete}
            />
          )}
        </Box>
      )}
      {editable && (
        <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#b45309", flexShrink: 0 }} />
            <Typography variant="body2" color="text.secondary">
              Under internal review - you can still add or remove files.
            </Typography>
          </Box>
          <EvidenceUploadBox
            auditId={auditId}
            controlId={controlId}
            phase="population"
            hint="CSV or XLSX complete list of in-scope items"
            buttonLabel="Add Files"
            onSubmitted={() => onStatusChange("POPULATION_INTERNAL_REVIEW")}
          />
        </Box>
      )}
    </SectionCard>
  );
}

function OEEvidenceSection({
  control,
  onStatusChange,
  canSubmitEvidence,
  canReviewEvidence,
  canManageControls,
  isAuditor,
}: {
  control: AuditControl;
  onStatusChange: (s: ControlStatus) => void;
  canSubmitEvidence: boolean;
  canReviewEvidence: boolean;
  canManageControls: boolean;
  isAuditor: boolean;
}): JSX.Element {
  const activeStep = oeActiveStep(control.status);
  // Population Submission is shown to everyone who can see the control, from
  // the first internal review all the way through Complete — not just the
  // team, and not just while it's still under review. It only stays out of
  // POPULATION_PENDING and POPULATION_NEED_CLARIFICATION, which already show
  // (now-empty, after a reject clears the round's files) files via their own
  // resubmit card.
  const showPopulationSubmissionCard =
    control.status !== "POPULATION_PENDING" && control.status !== "POPULATION_NEED_CLARIFICATION";
  // Files can only still be added/removed during internal review — once the
  // round is approved and moves to auditor validation it's locked, same as
  // teamEditablePopulationStatuses on the backend (population/handler.go).
  // Drives the upload box too, so it stays plain SubmitEvidence-gated: the
  // backend's uploadPopulation/submitPopulation routes have no ManageControls
  // bypass (only the delete routes do — see canDeletePopulationRecord below).
  const canEditPopulationFiles = canSubmitEvidence && control.status === "POPULATION_INTERNAL_REVIEW";
  // Removing a file or the note is allowed beyond the team-editable window
  // for ManageControls, mirroring deletePopulationFile/
  // deletePopulationAttestation's isAdmin bypass on the backend — an admin
  // cleaning up a note left over from a status override (round now locked,
  // so plain SubmitEvidence can't touch it) needs this even outside internal
  // review. Deliberately separate from canEditPopulationFiles so it never
  // also exposes the upload box, which admins aren't exempted for. COMPLETE
  // is excluded even for ManageControls — requireControlNotComplete on the
  // backend hard-locks it there regardless of privilege, so showing the
  // button would just produce a 409; an admin has to override the status off
  // COMPLETE first, which re-opens this on whatever earlier status it lands on.
  const canDeletePopulationRecord =
    control.status !== "COMPLETE" && (canEditPopulationFiles || canManageControls);

  return (
    <>
      <Paper variant="outlined" sx={{ borderRadius: 2, p: { xs: 1.5, sm: 2 }, overflow: "hidden" }}>
        <Stepper activeStep={activeStep} alternativeLabel sx={{ "& .MuiStepLabel-label": { fontSize: "0.72rem", mt: 0.5 } }}>
          {OE_STEPS.map((label) => (
            <Step key={label}><StepLabel>{label}</StepLabel></Step>
          ))}
        </Stepper>
      </Paper>

      {/* Population Submission — persistent, rendered once here rather than
          per-status below, so it never disappears once the auditor approves
          the population (see PopulationSubmissionCard). */}
      {showPopulationSubmissionCard && (
        <PopulationSubmissionCard
          auditId={control.auditId}
          controlId={control.id}
          editable={canEditPopulationFiles}
          canDelete={canDeletePopulationRecord}
          onStatusChange={onStatusChange}
        />
      )}

      {/* ── Step 0: Population phase ── */}
      {activeStep === 0 && (
        <>
          {control.status === "POPULATION_PENDING" && (
            <>
              {/* Internal-review reject lands back here (not a separate
                  clarification state — mirrors EVIDENCE_PENDING in the Design
                  flow). Population Requirement is shown on the Overview tab
                  instead (see ControlDrawer's Overview panel) — kept out of
                  this tab since it already has a lot going on. Files on
                  record (e.g. from before a reject, or an admin override back
                  to POPULATION_PENDING) live inside this same card via
                  SubmittedPopulationFiles rather than a separate card — same
                  layout as DesignEvidenceSection's Evidence Submission card. */}
              <SectionCard
                icon={<FileUp size={16} />}
                iconBg="transparent"
                title={control.comments ? "Resubmit Population" : "Submit Population"}
                flexContent
              >
                <SubmittedPopulationFiles auditId={control.auditId} controlId={control.id} rejectionReason={control.comments ?? null} />
                <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
                  <EvidenceUploadBox
                    auditId={control.auditId}
                    controlId={control.id}
                    phase="population"
                    hint="CSV or XLSX complete list of in-scope items"
                    buttonLabel={control.comments ? "Resubmit Population" : "Submit Population"}
                    onSubmitted={() => onStatusChange("POPULATION_INTERNAL_REVIEW")}
                  />
                </Box>
              </SectionCard>
            </>
          )}

          {control.status === "POPULATION_INTERNAL_REVIEW" && (
            <>
              {/* Population Submission itself is rendered once, above, for
                  every applicable status — not repeated here. AI Validation
                  sits right between that submission and the review decision
                  below (excluded from the general post-step-0 placement
                  further down, so it isn't rendered twice). Independent
                  blocks below it, not an either/or: an account that holds
                  both SubmitEvidence and ReviewEvidence (e.g. an admin, or
                  any allowAll/mock-auth account) must see the review decision
                  regardless of which file-management view it also sees. */}
              {canSubmitEvidence && (
                <AIValidationCard auditId={control.auditId} controlId={control.id} variant="submitter" phase="population" />
              )}
              {canReviewEvidence && (
                <PopulationReviewCard auditId={control.auditId} controlId={control.id} mode="review" onDecided={onStatusChange} />
              )}
              {!canReviewEvidence && (
                <SectionCard icon={<Clock size={16} />} iconBg="transparent" title="Population Under Internal Review">
                  <Box sx={{ py: 2, display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5, textAlign: "center" }}>
                    <Box sx={{ width: 52, height: 52, borderRadius: "50%", bgcolor: "#fff7ed", display: "flex", alignItems: "center", justifyContent: "center" }}>
                      <Clock size={24} color="#b45309" />
                    </Box>
                    <Typography variant="body2" fontWeight={600}>Population submitted successfully</Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ maxWidth: 320, lineHeight: 1.65 }}>
                      The compliance team is reviewing your population file before it goes to the auditor.
                    </Typography>
                    <Chip size="small" label="Pending internal review" sx={{ bgcolor: "#fff7ed", color: "#92400e", fontWeight: 500 }} />
                  </Box>
                </SectionCard>
              )}
            </>
          )}

          {control.status === "POPULATION_UNDER_VALIDATION" && (
            <>
              {/* Population Submission itself is rendered once, above (see
                  PopulationSubmissionCard) — read-only at this stage since
                  the round locks once internal review approves it. AI
                  Validation sits right between that submission and the
                  auditor's validation decision below (excluded from the
                  general post-step-0 placement further down, so it isn't
                  rendered twice — same pattern as POPULATION_INTERNAL_REVIEW
                  above). */}
              {canSubmitEvidence && (
                <AIValidationCard auditId={control.auditId} controlId={control.id} variant="submitter" phase="population" />
              )}
              {isAuditor ? (
                <PopulationReviewCard auditId={control.auditId} controlId={control.id} mode="validate" onDecided={onStatusChange} />
              ) : (
                <SectionCard icon={<Clock size={16} />} iconBg="transparent" title="Population Under Auditor Validation">
                  <Box sx={{ py: 2, display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5, textAlign: "center" }}>
                    <Box sx={{ width: 52, height: 52, borderRadius: "50%", bgcolor: "#f5f3ff", display: "flex", alignItems: "center", justifyContent: "center" }}>
                      <Clock size={24} color="#7c3aed" />
                    </Box>
                    <Typography variant="body2" fontWeight={600}>Population passed internal review</Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ maxWidth: 320, lineHeight: 1.65 }}>
                      The external auditor is reviewing your population and selecting a sample for evidence collection.
                    </Typography>
                    <Chip size="small" label="Waiting for auditor sample selection" sx={{ bgcolor: "#f5f3ff", color: "#6d28d9", fontWeight: 500 }} />
                  </Box>
                </SectionCard>
              )}
            </>
          )}

          {control.status === "POPULATION_NEED_CLARIFICATION" && (
            <>
              <SectionCard icon={<FileUp size={16} />} iconBg="transparent" title="Resubmit Population" flexContent>
                <SubmittedPopulationFiles auditId={control.auditId} controlId={control.id} />
                <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
                  <EvidenceUploadBox
                    auditId={control.auditId}
                    controlId={control.id}
                    phase="population"
                    hint="CSV or XLSX — complete list of in-scope items"
                    buttonLabel="Resubmit Population"
                    onSubmitted={() => onStatusChange("POPULATION_INTERNAL_REVIEW")}
                  />
                </Box>
              </SectionCard>
            </>
          )}
        </>
      )}

      {/* Population AI Validation — placeholder until the agent exists (see
          AIValidationCard's phase="population"). Placed after every
          population-submission surface above (the persistent card, and
          whichever step-0 sub-state is active) rather than before them, and
          scoped to the population phase only (the same set the Overview tab
          uses to decide which requirement text to show) so it steps aside
          once the job moves to evidence. POPULATION_INTERNAL_REVIEW and
          POPULATION_UNDER_VALIDATION are excluded here — they each get their
          own placement right before their review/validate decision instead
          (see above), between submission and that decision specifically,
          rather than after both. */}
      {canSubmitEvidence && OE_POPULATION_PHASE_STATUSES.has(control.status) && !POPULATION_REVIEW_STATUSES.has(control.status) && (
        <AIValidationCard auditId={control.auditId} controlId={control.id} variant="submitter" phase="population" />
      )}

      {/* ── Step 1a: Population approved → auditor selects the sample ── */}
      {(control.status === "POPULATION_COMPLETE" || control.status === "AWAITING_SAMPLE") && (
        isAuditor ? (
          <SampleUploadCard
            auditId={control.auditId}
            controlId={control.id}
            canRequestMoreTime={control.status === "POPULATION_COMPLETE"}
            onSubmitted={onStatusChange}
          />
        ) : (
          <SampleWaitingCard status={control.status} />
        )
      )}

      {/* ── Step 1b: Auditor selected the sample → team submits evidence.
          The auditor still sees an editable card here (not the read-only
          summary) so they can fix the sample right after submitting it. */}
      {control.status === "SUBMITTED_SAMPLE" && (
        <>
          {isAuditor ? (
            <SampleUploadCard
              auditId={control.auditId}
              controlId={control.id}
              canRequestMoreTime={false}
              editMode
              initialNote={control.sampleReference ?? ""}
              onSubmitted={onStatusChange}
            />
          ) : (
            <SampleSelectionCard auditId={control.auditId} controlId={control.id} sampleReference={control.sampleReference} />
          )}
          {canSubmitEvidence && (
            <SectionCard icon={<FileUp size={16} />} iconBg="transparent" title="Submit Evidence" flexContent>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5, lineHeight: 1.7 }}>
                Upload evidence covering all selected samples listed above.
              </Typography>
              {/* SUBMITTED_SAMPLE is normally evidence's first-ever stop, so
                  there is usually nothing to list here. But an admin override
                  can also land the control here from further along
                  (EVIDENCE_INTERNAL_REVIEW/NEED_CLARIFICATION/UNDER_VALIDATION)
                  — the cascade demotes that round's status without deleting
                  its files (see useOverrideControlStatus), so they're still on
                  record. Without this, they stayed invisible until a fresh
                  upload dragged them back into view merged with the new one. */}
              <SubmittedEvidenceList
                auditId={control.auditId}
                controlId={control.id}
                canDelete={canSubmitEvidence || canManageControls}
                onStatusChange={(s) => onStatusChange(s as ControlStatus)}
              />
              <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
                <EvidenceUploadBox
                  auditId={control.auditId}
                  controlId={control.id}
                  hint="PDF, XLSX, PNG up to 25 MB each"
                  buttonLabel="Submit Evidence"
                  onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
                />
              </Box>
            </SectionCard>
          )}
        </>
      )}

      {/* ── Step 2: Evidence rejected → resubmit ── */}
      {activeStep === 2 && (
        <>
          <SampleSelectionCard auditId={control.auditId} controlId={control.id} sampleReference={control.sampleReference} />
          <SectionCard icon={<FileUp size={16} />} iconBg="transparent" title="Resubmit Evidence" flexContent>
            {/* Same reasoning as DesignEvidenceSection's activeStep 0: the
                round left over from before the reject/override is still on
                record and was otherwise invisible until a fresh upload
                created a duplicate round. rejectionReason only when plain
                EVIDENCE_PENDING — this activeStep also covers
                EVIDENCE_NEED_CLARIFICATION, whose own status label already
                conveys "sent back". */}
            <SubmittedEvidenceList
              auditId={control.auditId}
              controlId={control.id}
              canDelete={canSubmitEvidence || canManageControls}
              rejectionReason={control.status === "EVIDENCE_PENDING" ? (control.comments ?? null) : undefined}
              onStatusChange={(s) => onStatusChange(s as ControlStatus)}
            />
            {canSubmitEvidence && (
              <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
                <EvidenceUploadBox
                  auditId={control.auditId}
                  controlId={control.id}
                  hint="PDF, XLSX, PNG up to 25 MB each"
                  buttonLabel="Resubmit Evidence"
                  onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
                />
              </Box>
            )}
          </SectionCard>
        </>
      )}

      {/* ── Step 3+: EVIDENCE_INTERNAL_REVIEW — show files + withdraw ── */}
      {activeStep >= 3 && control.status === "EVIDENCE_INTERNAL_REVIEW" && (
        <>
          <SampleSelectionCard auditId={control.auditId} controlId={control.id} sampleReference={control.sampleReference} />
          <SectionCard icon={<FileUp size={16} />} iconBg="transparent" title="Evidence Submission">
            <SubmittedEvidenceList
              auditId={control.auditId}
              controlId={control.id}
              canDelete={canSubmitEvidence || canManageControls}
              onStatusChange={(s) => onStatusChange(s as ControlStatus)}
            />
            {canSubmitEvidence && (
              <Box sx={{ mt: 2, pt: 2, borderTop: "1px solid", borderColor: "divider" }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
                  <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#b45309", flexShrink: 0 }} />
                  <Typography variant="body2" color="text.secondary">
                    Under internal review - you can still add or remove files.
                  </Typography>
                </Box>
                <EvidenceUploadBox
                  auditId={control.auditId}
                  controlId={control.id}
                  hint="PDF, XLSX, PNG up to 25 MB each"
                  buttonLabel="Add Files"
                  evidenceMode="append"
                  onSubmitted={() => onStatusChange("EVIDENCE_INTERNAL_REVIEW")}
                />
              </Box>
            )}
          </SectionCard>
        </>
      )}

      {/* ── Step 3+: EVIDENCE_UNDER_VALIDATION — files + inline status strip,
          same "Submitted Evidence" card Design uses at this stage (and the OE
          Complete block below), instead of a bare waiting-message card with
          no files in it. ── */}
      {activeStep >= 3 && control.status === "EVIDENCE_UNDER_VALIDATION" && (
        <>
          <SampleSelectionCard auditId={control.auditId} controlId={control.id} sampleReference={control.sampleReference} />
          <SectionCard icon={<ClipboardCheck size={16} />} iconBg="transparent" title="Submitted Evidence">
            {/* Locked for the team once the round reaches auditor validation —
                canDelete is ManageControls-only here, for the same admin
                cleanup case as a status override landing the control back on
                this step. */}
            <SubmittedEvidenceList auditId={control.auditId} controlId={control.id} canDelete={canManageControls} />
            <Box sx={{ mt: 1.5, py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "action.hover", display: "flex", alignItems: "center", gap: 1 }}>
              <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: "#7c3aed", flexShrink: 0 }} />
              <Typography variant="body2" color="text.secondary">Passed internal review. External auditor is validating.</Typography>
            </Box>
          </SectionCard>
        </>
      )}

      {/* ── Complete ── */}
      {control.status === "COMPLETE" && (
        <>
          <SampleSelectionCard auditId={control.auditId} controlId={control.id} sampleReference={control.sampleReference} />
          <SectionCard icon={<CheckCircle2 size={16} />} iconBg="transparent" title="Submitted Evidence">
            <SubmittedEvidenceList auditId={control.auditId} controlId={control.id} />
            <Box sx={{ mt: 1.5, py: 1, px: 1.5, borderRadius: 1.5, bgcolor: "rgba(22,163,74,0.06)", display: "flex", alignItems: "center", gap: 1 }}>
              <CheckCircle2 size={14} color="#16a34a" />
              <Typography variant="body2" color="text.secondary">
                {control.comments ?? "All evidence reviewed and approved by the auditor."}
              </Typography>
            </Box>
          </SectionCard>
        </>
      )}

      {/* Evidence AI Validation — fully wired (same evidence AI agent Design
          uses, see AIValidationCard's default phase="evidence"). Scoped to
          once the job has actually shifted to evidence (sample submitted
          onward) — showing it during the population/sample phases, before
          there is any evidence to validate, would just be premature clutter. */}
      {canSubmitEvidence && !OE_POPULATION_PHASE_STATUSES.has(control.status) && (
        <AIValidationCard auditId={control.auditId} controlId={control.id} variant="submitter" />
      )}
    </>
  );
}

// ─── Status override confirm dialog ────────────────────────────────────────────
// Selecting a target status in the (admin-only, editable) header chip does not
// commit it — this dialog shows the transition and cascade consequence
// explicitly and requires an extra confirm click, so a stray click never
// silently rewinds a control.

function OverrideStatusDialog({
  open,
  from,
  to,
  isPending,
  error,
  onConfirm,
  onClose,
}: {
  open: boolean;
  from: ControlStatus | null;
  to: ControlStatus | null;
  isPending: boolean;
  error: string | null;
  onConfirm: () => void;
  onClose: () => void;
}): JSX.Element {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ fontWeight: 700 }}>Override control status?</DialogTitle>
      <DialogContent>
        {error && <Alert severity="error" sx={{ mb: 1.5 }}>{error}</Alert>}
        <Typography variant="body2">
          This moves the control from <strong>{from ? CONTROL_STATUS_LABELS[from] : ""}</strong> to{" "}
          <strong>{to ? CONTROL_STATUS_LABELS[to] : ""}</strong>.
        </Typography>
      </DialogContent>
      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose} variant="outlined" disabled={isPending}>
          Cancel
        </Button>
        <Button
          onClick={onConfirm}
          variant="contained"
          disabled={isPending}
          startIcon={isPending ? <CircularProgress size={14} color="inherit" /> : undefined}
        >
          {isPending ? "Overriding…" : "Confirm Override"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

export default function ControlDrawer({ control, open, onClose }: ControlDrawerProps): JSX.Element {
  const { can } = useAuditPrivileges();
  const canSubmitEvidence = can(AuditPrivilege.SubmitEvidence);
  const canReviewEvidence = can(AuditPrivilege.ReviewEvidence);
  const canComment = can(AuditPrivilege.AddComment);
  const canManageControls = can(AuditPrivilege.ManageControls);
  const currentUserEmail = useCurrentUserEmail();
  // The assigned auditor POC (or an admin, who bypasses every gate the same way
  // on the backend) — drives population validation, sample selection, and
  // evidence validation across both DESIGN and OE controls.
  const isAuditor = Boolean(control) && (isAssignedAuditor(control as AuditControl, currentUserEmail) || canManageControls);
  const validateEvidence = useValidateEvidence();
  const reviewEvidence = useReviewEvidence();
  const overrideStatus = useOverrideControlStatus();

  const [tab, setTab] = useState(0);
  const [localStatus, setLocalStatus] = useState<{ id: number; status: ControlStatus } | null>(null);
  const [overrideTarget, setOverrideTarget] = useState<ControlStatus | null>(null);

  // Reset to the Overview tab whenever a different control is opened, so the
  // drawer doesn't retain the previous control's active tab. Syncing tab state to
  // the opened control is a legitimate effect here.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTab(0);
  }, [control?.id]);
  // Use local override only when it belongs to the currently open control
  const displayStatus =
    localStatus !== null && control !== null && localStatus.id === control.id
      ? localStatus.status
      : control?.status;

  // Reflect a transition the backend has *already* applied (evidence submit,
  // withdraw, deleting the last file). These must not PATCH /status: that
  // endpoint needs ManageControls, which a submitter does not hold, and the
  // resulting 403 would roll the optimistic value back and strand the drawer on
  // a stale step. The invalidated controls query supplies the authoritative
  // value a moment later.
  function applyServerStatus(c: AuditControl, newStatus: ControlStatus) {
    setLocalStatus({ id: c.id, status: newStatus });
  }

  function handleConfirmOverride() {
    if (!control || !overrideTarget) return;
    overrideStatus.mutate(
      { auditId: control.auditId, controlId: control.id, status: overrideTarget },
      {
        onSuccess: () => {
          applyServerStatus(control, overrideTarget);
          setOverrideTarget(null);
        },
      },
    );
  }

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: "100vw", sm: 660, md: 720 },
          display: "flex",
          flexDirection: "column",
        },
      }}
    >
      {control && (
        // key resets all local state when a different control is opened
        <Box key={control.id} sx={{ display: "flex", flexDirection: "column", height: "100%" }}>

          {/* ── Header ── */}
          <Box sx={{ px: 3, pt: 2.5, pb: 2, flexShrink: 0 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: 1,
                mb: 1.25,
              }}
            >
              <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                <Typography variant="h5" fontWeight={700}>
                  {control.controlNumber}
                </Typography>
                <ControlStatusChip
                  status={displayStatus ?? control.status}
                  size="medium"
                  editable={canManageControls}
                  requirementType={control.requirementType as "DESIGN" | "OE"}
                  onOverride={setOverrideTarget}
                />
                {control.isOverdue && (
                  <Chip
                    icon={<AlertCircle size={13} />}
                    label="Overdue"
                    size="small"
                    variant="outlined"
                    sx={{ color: "#dc2626", borderColor: "#dc2626", fontWeight: 500 }}
                  />
                )}
              </Stack>
              <IconButton size="small" onClick={onClose} aria-label="Close">
                <X size={18} />
              </IconButton>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.7 }}>
              {control.description}
            </Typography>
          </Box>

          {/* ── Tabs ── */}
          <Tabs
            value={tab}
            onChange={(_, v: number) => setTab(v)}
            sx={{ px: 2, borderBottom: 1, borderTop: 1, borderColor: "divider", flexShrink: 0, minHeight: 44 }}
          >
            <Tab
              icon={<ClipboardCheck size={15} />}
              iconPosition="start"
              label="Overview"
              sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }}
            />
            <Tab
              icon={<FileUp size={15} />}
              iconPosition="start"
              label="Evidence"
              sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }}
            />
            <Tab
              icon={<History size={15} />}
              iconPosition="start"
              label="History"
              sx={{ textTransform: "none", minHeight: 44, fontWeight: 600 }}
            />
          </Tabs>

          {/* ══ TAB 0 – OVERVIEW ══════════════════════════════════════════════ */}
          <TabPanel value={tab} index={0}>

            {/* Control details — fixed 3-column grid (not a wrapping flex
                row): every field keeps the same cell every time, so a
                returning user can find e.g. Due Date in the same spot
                regardless of drawer width or value length. 3 columns instead
                of the old 2 keeps the same recognizable layout while taking
                less vertical space. */}
            <SectionCard
              icon={<ClipboardCheck size={16} />}
              iconBg="transparent"
              title="Control Details"
            >
              <Box sx={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 1 }}>

                <InfoTile label="Requirement Type">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                    {REQ_TYPE_LABELS[control.requirementType]}
                  </Typography>
                </InfoTile>

                <InfoTile label="Control Type">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                    {CTRL_TYPE_LABELS[control.controlType]}
                  </Typography>
                </InfoTile>

                <InfoTile label="Scope">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                    {SCOPE_LABELS[control.scope]}
                  </Typography>
                </InfoTile>

                <DueDateTile label="Due Date" date={control.dueDate} overdue={control.isOverdue} />

                <InfoTile label="Team">
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Users size={13} style={{ flexShrink: 0, opacity: 0.55 }} />
                    <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                      {control.teamName ?? "—"}
                    </Typography>
                  </Box>
                </InfoTile>

                <InfoTile label="Sample Reference">
                  <Typography variant="body2" fontWeight={600} fontSize="0.8rem" noWrap>
                    {control.sampleReference ?? "—"}
                  </Typography>
                </InfoTile>

                <InfoTile label="Process Owner">
                  {control.ownerName ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                      <UserAvatar name={control.ownerName} size={22} />
                      <Typography variant="body2" fontWeight={600} fontSize="0.8rem" noWrap>
                        {control.ownerName}
                      </Typography>
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.disabled" fontSize="0.8rem">—</Typography>
                  )}
                </InfoTile>

                <InfoTile label="Auditor POC">
                  {control.auditorName ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                      <UserAvatar name={control.auditorName} size={22} />
                      <Typography variant="body2" fontWeight={600} fontSize="0.8rem" noWrap>
                        {control.auditorName}
                      </Typography>
                    </Box>
                  ) : (
                    <Typography variant="body2" color="text.disabled" fontSize="0.8rem">—</Typography>
                  )}
                </InfoTile>

              </Box>
            </SectionCard>

            {/* Population Details — OE only, its own box separate from Control
                Details above, same fixed-grid style. Population Comments is
                rendered as a plain text block below the grid rather than a
                tile: comments can run long, and a small tile clips or crams
                that kind of free text. */}
            {control.requirementType === "OE" && (
              <SectionCard
                icon={<Users size={16} />}
                iconBg="transparent"
                title="Population Details"
              >
                <Box sx={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 1 }}>
                  <DueDateTile label="Population Due Date" date={control.populationDueDate ?? null} />

                  <InfoTile label="Population Owner">
                    {control.populationOwnerName ? (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                        <UserAvatar name={control.populationOwnerName} size={22} />
                        <Typography variant="body2" fontWeight={600} fontSize="0.8rem" noWrap>
                          {control.populationOwnerName}
                        </Typography>
                      </Box>
                    ) : (
                      <Typography variant="body2" color="text.disabled" fontSize="0.8rem">—</Typography>
                    )}
                  </InfoTile>

                  <InfoTile label="Population Team">
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                      <Users size={13} style={{ flexShrink: 0, opacity: 0.55 }} />
                      <Typography variant="body2" fontWeight={600} fontSize="0.8rem">
                        {control.populationTeamName ?? "—"}
                      </Typography>
                    </Box>
                  </InfoTile>
                </Box>

                {control.populationComments && (
                  <Box sx={{ mt: 1.5, pt: 1.5, borderTop: "1px solid", borderColor: "divider" }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: "block", mb: 0.5 }}>
                      Population Comments
                    </Typography>
                    <Typography variant="body2" sx={{ lineHeight: 1.7 }}>
                      {control.populationComments}
                    </Typography>
                  </Box>
                )}
              </SectionCard>
            )}

            {/* Requirement text — for Design controls this is always the
                Evidence Requirement. For OE controls it shows the Population
                Requirement while the control is still in the population phase,
                then switches to the Evidence Requirement once the auditor has
                submitted the sample (the team's job has moved from describing
                the population to providing evidence). Kept here in Overview
                rather than the Evidence tab, which already has a lot going on. */}
            {(() => {
              const status = displayStatus ?? control.status;
              const showPopulationRequirement =
                control.requirementType === "OE" && OE_POPULATION_PHASE_STATUSES.has(status);
              const requirementText = showPopulationRequirement
                ? control.populationDescription
                : control.evidenceRequirement;
              if (!requirementText) return null;
              return (
                <SectionCard
                  icon={<FileText size={16} />}
                  iconBg="transparent"
                  title={showPopulationRequirement ? "Population Requirement" : "Evidence Requirement"}
                >
                  <Typography variant="body2" sx={{ lineHeight: 1.8 }}>
                    {requirementText}
                  </Typography>
                </SectionCard>
              );
            })()}

          </TabPanel>

          {/* ══ TAB 1 – EVIDENCE ══════════════════════════════════════════════ */}
          <TabPanel value={tab} index={1}>

            {control.requirementType === "OE" ? (
              <OEEvidenceSection
                control={{ ...control, status: displayStatus ?? control.status }}
                onStatusChange={(s) => applyServerStatus(control, s)}
                canSubmitEvidence={canSubmitEvidence}
                canReviewEvidence={canReviewEvidence}
                canManageControls={canManageControls}
                isAuditor={isAuditor}
              />
            ) : (
              <DesignEvidenceSection
                control={{ ...control, status: displayStatus ?? control.status }}
                onStatusChange={(s) => applyServerStatus(control, s)}
                canSubmitEvidence={canSubmitEvidence}
                canManageControls={canManageControls}
              />
            )}

            {/* AI pre-review hint (advisory) above the reviewer's decision */}
            {canReviewEvidence && (
              <AIValidationCard auditId={control.auditId} controlId={control.id} variant="reviewer" />
            )}

            {/* Internal Review — shown only for the exact window it's
                actionable in (EVIDENCE_INTERNAL_REVIEW): not before evidence
                exists, and not once the round has moved past internal review,
                so there's no lingering card with nothing to do. */}
            {canReviewEvidence && displayStatus === "EVIDENCE_INTERNAL_REVIEW" && (
            <SectionCard
              icon={<ClipboardCheck size={16} />}
              iconBg="transparent"
              title="Internal Review"
            >
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                <Button
                  variant="contained"
                  disableElevation
                  disabled={reviewEvidence.isPending}
                  startIcon={reviewEvidence.isPending ? <CircularProgress size={15} color="inherit" /> : <CheckCircle2 size={15} />}
                  onClick={() => reviewEvidence.mutate(
                    { auditId: control.auditId, controlId: control.id, decision: "APPROVE" },
                    { onSuccess: (data) => applyServerStatus(control, data.status as ControlStatus) },
                  )}
                  sx={{ textTransform: "none", fontWeight: 600, bgcolor: "#b45309", color: "#fff", "&:hover": { bgcolor: "#92400e" } }}
                >
                  Approve
                </Button>
                <Button
                  variant="outlined"
                  disabled={reviewEvidence.isPending}
                  startIcon={<XCircle size={15} />}
                  onClick={() => reviewEvidence.mutate(
                    { auditId: control.auditId, controlId: control.id, decision: "REJECT" },
                    { onSuccess: (data) => applyServerStatus(control, data.status as ControlStatus) },
                  )}
                  sx={{ textTransform: "none", fontWeight: 600, color: "#dc2626", borderColor: "#dc2626", "&:hover": { borderColor: "#b91c1c", bgcolor: "rgba(220,38,38,0.04)" } }}
                >
                  Reject
                </Button>
              </Box>
              {reviewEvidence.isError && (
                <Alert severity="error" sx={{ mt: 1, fontSize: "0.8rem" }}>{(reviewEvidence.error as Error).message}</Alert>
              )}
            </SectionCard>
            )}

            {/* Auditor Validation — the assigned auditor POC only (or admin), not
                every REVIEW_EVIDENCE holder: this is the external-auditor decision,
                distinct from the Internal Review card above. Same rule: shown
                only during EVIDENCE_UNDER_VALIDATION, not before or after. */}
            {isAuditor && displayStatus === "EVIDENCE_UNDER_VALIDATION" && (
            <SectionCard
              icon={<ClipboardCheck size={16} />}
              iconBg="transparent"
              title="Auditor Validation"
            >
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                <Button
                  variant="contained"
                  disableElevation
                  disabled={validateEvidence.isPending}
                  startIcon={validateEvidence.isPending ? <CircularProgress size={15} color="inherit" /> : <CheckCircle2 size={15} />}
                  onClick={() => validateEvidence.mutate(
                    { auditId: control.auditId, controlId: control.id, decision: "APPROVE" },
                    { onSuccess: (data) => applyServerStatus(control, data.status as ControlStatus) },
                  )}
                  sx={{ textTransform: "none", fontWeight: 600, bgcolor: "#7c3aed", color: "#fff", "&:hover": { bgcolor: "#6d28d9" } }}
                >
                  Approve
                </Button>
                <Button
                  variant="outlined"
                  disabled={validateEvidence.isPending}
                  startIcon={<RotateCcw size={15} />}
                  onClick={() => validateEvidence.mutate(
                    { auditId: control.auditId, controlId: control.id, decision: "REJECT" },
                    { onSuccess: (data) => applyServerStatus(control, data.status as ControlStatus) },
                  )}
                  sx={{ textTransform: "none", fontWeight: 600 }}
                >
                  Request Resubmission
                </Button>
              </Box>
              {validateEvidence.isError && (
                <Alert severity="error" sx={{ mt: 1, fontSize: "0.8rem" }}>{(validateEvidence.error as Error).message}</Alert>
              )}
            </SectionCard>
            )}

            {/* Comments */}
            <SectionCard
              icon={<MessageSquare size={16} />}
              iconBg="transparent"
              title="Comments"
            >
              <CommentsSection auditId={control.auditId} controlId={control.id} canComment={canComment} />
            </SectionCard>

          </TabPanel>

          {/* ══ TAB 2 – HISTORY ═══════════════════════════════════════════════ */}
          <TabPanel value={tab} index={2}>
            <ControlHistoryTimeline
              auditId={control.auditId}
              controlId={control.id}
              currentStatus={displayStatus ?? control.status}
            />
          </TabPanel>

        </Box>
      )}

      <OverrideStatusDialog
        open={overrideTarget !== null}
        from={control?.status ?? null}
        to={overrideTarget}
        isPending={overrideStatus.isPending}
        error={overrideStatus.isError ? (overrideStatus.error as Error).message : null}
        onConfirm={handleConfirmOverride}
        onClose={() => setOverrideTarget(null)}
      />
    </Drawer>
  );
}

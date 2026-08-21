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

import { Alert, Box, Button, CircularProgress, IconButton, Skeleton, Typography } from "@wso2/oxygen-ui";
import { Download, ExternalLink, FileText, RotateCcw, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useGetEvidence, evidenceQueryKey } from "@modules/audit/api/useGetEvidence";
import { controlsQueryKey } from "@modules/audit/api/useGetControls";
import { aiValidationQueryKey } from "@modules/audit/api/useGetAIValidation";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import { formatTimestamp } from "@modules/audit/utils/format";
import { downloadBlob, viewOrDownloadBlob } from "@modules/audit/utils/fileView";
import { extractErrorMessage } from "@modules/audit/api/apiError";

function sizeLabel(bytes: number | null): string {
  if (bytes === null) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * Lists the files a team submitted for a control so they can be viewed/downloaded.
 * Pass `canDelete` to show per-file remove buttons (submitter view at step 1).
 *
 * Deleting the last file empties the submission, so the backend sends the control
 * back to EVIDENCE_PENDING; `onStatusChange` lets the caller reflect that without
 * waiting for a refetch.
 *
 * Pass `rejectionReason` only at call sites rendering while control.status is
 * plain EVIDENCE_PENDING — internal-review reject and a status override both
 * land there (same rank as a brand-new control, see controlStatusRank in the
 * entity), so unlike EVIDENCE_NEED_CLARIFICATION (whose name already says
 * "this was sent back"), EVIDENCE_PENDING alone can't tell a first-time
 * submission from a resubmission. Pass `control.comments ?? null`; never pass
 * it at all for EVIDENCE_NEED_CLARIFICATION, whose status label already
 * covers this. Undefined skips the note outright; null still shows it (with
 * generic wording) as long as a prior round's files are on record.
 */
export default function SubmittedEvidenceList({
  auditId,
  controlId,
  canDelete = false,
  rejectionReason,
  onStatusChange,
}: {
  auditId: number;
  controlId: number;
  canDelete?: boolean;
  rejectionReason?: string | null;
  onStatusChange?: (status: string) => void;
}): JSX.Element {
  const { data, isLoading, isError } = useGetEvidence(auditId, controlId, true);
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  async function handleView(readUrl: string, fileName: string): Promise<void> {
    setDownloadError(null);
    try {
      const res = await authFetch(`${BACKEND_BASE_URL}${readUrl}`);
      if (!res.ok) throw new Error(`Download failed (${res.status})`);
      const blob = await res.blob();
      viewOrDownloadBlob(blob, fileName);
    } catch (err) {
      setDownloadError(err instanceof Error ? err.message : "Failed to download file");
    }
  }

  async function handleDownload(readUrl: string, fileName: string): Promise<void> {
    setDownloadError(null);
    try {
      const res = await authFetch(`${BACKEND_BASE_URL}${readUrl}`);
      if (!res.ok) throw new Error(`Download failed (${res.status})`);
      const blob = await res.blob();
      downloadBlob(blob, fileName);
    } catch (err) {
      setDownloadError(err instanceof Error ? err.message : "Failed to download file");
    }
  }

  async function handleDelete(fileId: number, evidenceId: number): Promise<void> {
    setDeleteError(null);
    setDeletingId(fileId);
    try {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/evidence/files/${fileId}`,
        { method: "DELETE" },
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to remove file (${res.status})`));
      }
      // The backend reports the control status after the delete: removing the
      // last file sends the submission back to EVIDENCE_PENDING.
      const { status } = (await res.json().catch(() => ({}))) as { status?: string };
      await queryClient.invalidateQueries({ queryKey: evidenceQueryKey(auditId, controlId) });
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      void queryClient.invalidateQueries({ queryKey: aiValidationQueryKey(evidenceId) });
      if (status) onStatusChange?.(status);
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Failed to remove file");
    } finally {
      setDeletingId(null);
    }
  }

  async function handleDeleteRound(evidenceId: number): Promise<void> {
    setDeleteError(null);
    setDeletingId(evidenceId);
    try {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/evidence/${evidenceId}`,
        { method: "DELETE" },
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to remove submission (${res.status})`));
      }
      const { status } = (await res.json().catch(() => ({}))) as { status?: string };
      await queryClient.invalidateQueries({ queryKey: evidenceQueryKey(auditId, controlId) });
      void queryClient.invalidateQueries({ queryKey: controlsQueryKey(auditId) });
      if (status) onStatusChange?.(status);
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Failed to remove submission");
    } finally {
      setDeletingId(null);
    }
  }

  if (isLoading) {
    return <Skeleton variant="rectangular" height={56} sx={{ borderRadius: 1 }} />;
  }
  if (isError) {
    return <Typography variant="body2" color="error">Failed to load submitted evidence.</Typography>;
  }

  // A resubmission creates a new round, and deleting a round's last file leaves
  // an otherwise-empty round behind. Drop rounds with no files so the list shows
  // one "Submitted …" header per round that actually holds evidence.
  //
  // Also drop rounds a reviewer/auditor already rejected (COMPLIANCE_REJECTED/
  // AUDITOR_REJECTED, set by useReviewEvidence/useValidateEvidence, or by the
  // admin override cascade into EVIDENCE_NEED_CLARIFICATION) — but only once a
  // newer round has actually superseded it (index > 0 in `data`, which is
  // newest first): showing a superseded rejected round alongside a fresh
  // resubmission would conflate old, no-longer-relevant files with it. While a
  // rejected round is still the latest one (index 0 — the reject just
  // happened, or an override just landed, and nothing has been resubmitted
  // yet), it stays visible here so the team can see, delete, and replace it;
  // once resubmitted it drops out and remains visible only in the History tab.
  const REJECTED_STATUSES = new Set(["COMPLIANCE_REJECTED", "AUDITOR_REJECTED"]);
  const allRounds = data ?? [];
  const submissions = allRounds.filter((s, i) => {
    const hasContent = (s.files?.length ?? 0) > 0 || Boolean(s.attestation);
    if (!hasContent) return false;
    // "Superseded" means a newer round actually has content — not just a
    // lower array index, since a resubmission's files can later be deleted
    // and leave a newer, empty round in front of this one (see hasContent
    // above, and the comment block up top).
    const hasNewerContent = allRounds
      .slice(0, i)
      .some((round) => (round.files?.length ?? 0) > 0 || Boolean(round.attestation));
    if (REJECTED_STATUSES.has(s.status) && hasNewerContent) return false;
    return true;
  });

  // Only note a resubmission when this call site opted in (rejectionReason
  // passed, meaning control.status is plain EVIDENCE_PENDING) and there is
  // something to resubmit — either a reason was given, or a prior round's
  // files are still on record (a reject with no comment, or a status
  // override). A brand-new control at EVIDENCE_PENDING with neither has
  // nothing to flag as "sent back".
  const showResubmissionNote = rejectionReason !== undefined && (submissions.length > 0 || Boolean(rejectionReason));
  const resubmissionNote = showResubmissionNote && (
    <Box sx={{ display: "flex", alignItems: "flex-start", gap: 0.75 }}>
      <RotateCcw size={13} color="#b45309" style={{ flexShrink: 0, marginTop: 2 }} />
      <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.6 }}>
        {rejectionReason ? `Sent back for revision: ${rejectionReason}` : "Resubmit to continue."}
      </Typography>
    </Box>
  );

  if (submissions.length === 0) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
        {resubmissionNote}
        <Typography variant="body2" color="text.secondary">No evidence files submitted yet.</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      {resubmissionNote}
      {(downloadError ?? deleteError) && (
        <Alert
          severity="error"
          onClose={() => { setDownloadError(null); setDeleteError(null); }}
          sx={{ fontSize: "0.8rem" }}
        >
          {downloadError ?? deleteError}
        </Alert>
      )}
      {submissions.map((sub) => (
        <Box key={sub.id} sx={{ display: "flex", flexDirection: "column", gap: 0.75 }}>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
            Submitted {formatTimestamp(sub.createdAt)}{(sub.createdByName || sub.createdBy) ? ` · ${sub.createdByName || sub.createdBy}` : ""}
          </Typography>
          {(sub.files?.length ?? 0) === 0 && sub.attestation && (
            <Box
              sx={{ display: "flex", alignItems: "flex-start", gap: 1, px: 1.25, py: 0.85, borderRadius: 1, border: "1px solid", borderColor: "divider", bgcolor: "action.hover" }}
            >
              <FileText size={15} style={{ flexShrink: 0, marginTop: 2 }} />
              <Box sx={{ flex: 1 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, display: "block" }}>
                  Completed without files.
                </Typography>
                <Typography variant="body2" sx={{ lineHeight: 1.6 }}>{sub.attestation}</Typography>
              </Box>
              {canDelete && (
                <IconButton
                  size="small"
                  aria-label="Remove submission"
                  disabled={deletingId !== null}
                  onClick={() => { void handleDeleteRound(sub.id); }}
                  sx={{ p: 0.5, color: "error.main", "&:hover": { bgcolor: "rgba(220,38,38,0.06)" } }}
                >
                  {deletingId === sub.id
                    ? <CircularProgress size={13} color="inherit" />
                    : <Trash2 size={14} />}
                </IconButton>
              )}
            </Box>
          )}
          {(sub.files ?? []).map((f) => (
            <Box
              key={f.id}
              sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.25, py: 0.85, borderRadius: 1, border: "1px solid", borderColor: "divider", bgcolor: "action.hover" }}
            >
              <FileText size={15} />
              <Typography variant="body2" sx={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {f.fileName}
              </Typography>
              {f.fileSize !== null && (
                <Typography variant="caption" color="text.secondary">{sizeLabel(f.fileSize)}</Typography>
              )}
              {f.readUrl ? (
                <>
                  <Button
                    size="small"
                    onClick={() => { void handleView(f.readUrl as string, f.fileName); }}
                    startIcon={<ExternalLink size={13} />}
                    sx={{ textTransform: "none", minWidth: 0 }}
                  >
                    View
                  </Button>
                  <IconButton
                    size="small"
                    aria-label={`Download ${f.fileName}`}
                    onClick={() => { void handleDownload(f.readUrl as string, f.fileName); }}
                    sx={{ p: 0.5 }}
                  >
                    <Download size={14} />
                  </IconButton>
                </>
              ) : (
                <Typography variant="caption" color="text.disabled">unavailable</Typography>
              )}
              {canDelete && (
                <IconButton
                  size="small"
                  aria-label={`Remove ${f.fileName}`}
                  disabled={deletingId !== null}
                  onClick={() => { void handleDelete(f.id, sub.id); }}
                  sx={{ p: 0.5, color: "error.main", "&:hover": { bgcolor: "rgba(220,38,38,0.06)" } }}
                >
                  {deletingId === f.id
                    ? <CircularProgress size={13} color="inherit" />
                    : <Trash2 size={14} />}
                </IconButton>
              )}
            </Box>
          ))}
        </Box>
      ))}
    </Box>
  );
}

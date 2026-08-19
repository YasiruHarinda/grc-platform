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

import { Alert, Box, Button, CircularProgress, IconButton, TextField, Typography } from "@wso2/oxygen-ui";
import { FileUp, Upload, X } from "@wso2/oxygen-ui-icons-react";
import { useRef, useState, type JSX } from "react";
import { useSubmitEvidence } from "@modules/audit/api/useSubmitEvidence";
import { useAddEvidenceFiles } from "@modules/audit/api/useAddEvidenceFiles";
import { useSubmitPopulation } from "@modules/audit/api/useSubmitPopulation";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";

/**
 * Largest file the backend accepts per upload request. Must stay in sync with
 * maxEvidenceUploadBytes in internal/audit/handler/evidence.go — checking here
 * turns a late 413 into an immediate, nameable error. The cap is per file, not
 * per submission: each file is uploaded in its own request.
 */
const MAX_FILE_BYTES = 25 * 1024 * 1024;

/**
 * UX hint only — narrows the file picker to expected evidence formats. The
 * backend is the actual security boundary: it rejects HTML/SVG/XML/JS
 * uploads outright (see validateUploadFileType), since those can execute as
 * script if ever rendered instead of downloaded.
 */
const ACCEPTED_FILE_TYPES =
  ".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.csv,.txt,.png,.jpg,.jpeg,.gif,.webp,.zip,.msg,.eml";

interface EvidenceUploadBoxProps {
  auditId: number;
  controlId: number;
  hint: string;
  buttonLabel: string;
  // Called after a successful submission (e.g. to advance the stepper).
  onSubmitted: () => void;
  /** Which submission flow to drive; defaults to the evidence endpoints. */
  phase?: "evidence" | "population";
  /**
   * evidence phase only. "append" (the "Add Files" case, used while a
   * submission is still under internal review) adds to the CURRENT round via
   * useAddEvidenceFiles instead of starting a brand-new one — starting a new
   * round here would leave the still-open one stranded once a reviewer's
   * decision only closes out the latest round, silently resurfacing its files
   * alongside every future resubmission. Defaults to "new" (the initial
   * submission / post-rejection resubmission case).
   */
  evidenceMode?: "new" | "append";
}

/**
 * Manual evidence submission box: pick/drop files and upload them via the SAS
 * flow (useSubmitEvidence). This is the primary submission path on the platform;
 * the evidence agent uses the same backend endpoints programmatically. With
 * phase="population" the same UI drives the population submission endpoints —
 * both flows require at least one file before Submit enables.
 */
export default function EvidenceUploadBox({
  auditId,
  controlId,
  hint,
  buttonLabel,
  onSubmitted,
  phase = "evidence",
  evidenceMode = "new",
}: EvidenceUploadBoxProps): JSX.Element {
  const [files, setFiles] = useState<File[]>([]);
  const [sizeError, setSizeError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [attestation, setAttestation] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const submitEvidence = useSubmitEvidence();
  const addEvidenceFiles = useAddEvidenceFiles();
  const submitPopulation = useSubmitPopulation();
  const { can } = useAuditPrivileges();
  const submit =
    phase === "population" ? submitPopulation
    : evidenceMode === "append" ? addEvidenceFiles
    : submitEvidence;
  const busy = submit.isPending;
  // Fileless completion, gated differently per phase: evidence restricts it to
  // ManageControls admins (a control-level exception); population is open to
  // whoever can submit population files at all, matching sample selection's
  // own "files, a note, or both" rule one step later in the same OE flow (no
  // extra privilege check there either). Neither applies to "Add Files".
  const allowAttestation =
    evidenceMode === "new" &&
    (phase === "population" || (phase === "evidence" && can(AuditPrivilege.ManageControls)));

  function addFiles(list: FileList | null) {
    if (!list) return;
    const incoming = Array.from(list);

    const tooBig = incoming.filter((f) => f.size > MAX_FILE_BYTES);
    setSizeError(
      tooBig.length === 0
        ? null
        : `${tooBig.map((f) => f.name).join(", ")} — each file must be 25 MB or smaller.`,
    );

    const accepted = incoming.filter((f) => f.size <= MAX_FILE_BYTES);
    setFiles((prev) => {
      const seen = new Set(prev.map((f) => f.name + f.size));
      return [...prev, ...accepted.filter((f) => !seen.has(f.name + f.size))];
    });
  }

  function removeFile(idx: number) {
    setFiles((prev) => prev.filter((_, i) => i !== idx));
  }

  function handleSubmit() {
    const onDone = { onSuccess: () => { setFiles([]); setSizeError(null); setAttestation(""); onSubmitted(); } };
    if (allowAttestation && files.length === 0) {
      if (phase === "population") {
        submitPopulation.mutate({ auditId, controlId, files, attestation }, onDone);
      } else {
        submitEvidence.mutate({ auditId, controlId, files, attestation }, onDone);
      }
      return;
    }
    submit.mutate({ auditId, controlId, files }, onDone);
  }

  const fileless = allowAttestation && files.length === 0;
  const submitDisabled = fileless ? attestation.trim() === "" || busy : files.length === 0 || busy;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", flex: 1 }}>
      <input
        ref={inputRef}
        type="file"
        multiple
        hidden
        accept={ACCEPTED_FILE_TYPES}
        onChange={(e) => { addFiles(e.target.files); e.target.value = ""; }}
      />

      <Box
        role="button"
        tabIndex={busy ? -1 : 0}
        aria-label="Upload files — click or press Enter to browse"
        onClick={() => !busy && inputRef.current?.click()}
        onKeyDown={(e) => { if (!busy && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); inputRef.current?.click(); } }}
        onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => { e.preventDefault(); setDragOver(false); if (!busy) addFiles(e.dataTransfer.files); }}
        sx={(theme) => ({
          border: "2px dashed",
          borderColor: dragOver ? "primary.main" : theme.palette.mode === "dark" ? "rgba(255,255,255,0.15)" : "#d1d5db",
          bgcolor: dragOver ? "action.hover" : "transparent",
          borderRadius: 2,
          p: 3,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          gap: 1,
          cursor: busy ? "default" : "pointer",
          textAlign: "center",
          mb: 1.5,
          "&:hover": { borderColor: "primary.main", bgcolor: "action.hover" },
        })}
      >
        <Box sx={{ width: 44, height: 44, borderRadius: "50%", display: "flex", alignItems: "center", justifyContent: "center", color: "text.secondary" }}>
          <Upload size={20} />
        </Box>
        <Typography variant="body2" fontWeight={600}>Drop files here or click to browse</Typography>
        <Typography variant="caption" color="text.secondary">{hint}</Typography>
      </Box>

      {files.length > 0 && (
        <Box sx={{ mb: 1.5, display: "flex", flexDirection: "column", gap: 0.5 }}>
          {files.map((f, i) => (
            <Box key={f.name + f.size + i} sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.25, py: 0.75, borderRadius: 1, bgcolor: "action.hover" }}>
              <FileUp size={14} />
              <Typography variant="caption" sx={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {f.name}
              </Typography>
              <Typography variant="caption" color="text.secondary">{(f.size / 1024).toFixed(0)} KB</Typography>
              <IconButton size="small" aria-label={`Remove ${f.name}`} disabled={busy} onClick={(e) => { e.stopPropagation(); removeFile(i); }} sx={{ p: 0.25 }}>
                <X size={13} />
              </IconButton>
            </Box>
          ))}
        </Box>
      )}

      {sizeError && (
        <Alert severity="warning" onClose={() => setSizeError(null)} sx={{ mb: 1.5, fontSize: "0.8rem" }}>
          {sizeError}
        </Alert>
      )}

      {allowAttestation && files.length === 0 && (
        <TextField
          multiline
          minRows={2}
          placeholder={
            phase === "population"
              ? "Written note explaining why there are no population files (required)"
              : "Written justification for completing this control with no files (required)"
          }
          value={attestation}
          onChange={(e) => setAttestation(e.target.value)}
          disabled={busy}
          fullWidth
          size="small"
          sx={{ mb: 1.5 }}
        />
      )}

      {(submit.isError || (fileless && phase === "evidence" && submitEvidence.isError)) && (
        <Alert severity="error" sx={{ mb: 1.5, fontSize: "0.8rem" }}>
          {((fileless && phase === "evidence" ? submitEvidence.error : submit.error) as Error).message}
        </Alert>
      )}

      <Button
        variant="contained"
        fullWidth
        disableElevation
        startIcon={busy ? <CircularProgress size={15} color="inherit" /> : <FileUp size={15} />}
        disabled={submitDisabled}
        onClick={handleSubmit}
        sx={{ textTransform: "none", fontWeight: 600 }}
      >
        {busy ? "Uploading…" : buttonLabel}
      </Button>
    </Box>
  );
}

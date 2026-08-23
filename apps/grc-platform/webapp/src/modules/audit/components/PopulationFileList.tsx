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

import { Alert, Box, Button, CircularProgress, IconButton, Typography } from "@wso2/oxygen-ui";
import { Download, ExternalLink, FileText, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import type { PopulationFile } from "@modules/audit/api/useGetPopulation";
import { useDeletePopulationFile } from "@modules/audit/api/useDeletePopulationFile";
import { downloadBlob, viewOrDownloadBlob } from "@modules/audit/utils/fileView";
import { formatTimestamp } from "@modules/audit/utils/format";

function sizeLabel(bytes: number | null): string {
  if (bytes === null) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * List of population/sample files — view and download always; pass
 * `onDelete` (with `auditId`/`controlId`) to also show a per-file remove
 * button, for the states where the caller is still editing the round
 * (resubmission, or the auditor updating a submitted sample).
 *
 * `attributionLabel` opens each file's own header line ("Submitted" for team
 * population files, "Uploaded" for the auditor's sample) — per file, not
 * batched, since there's no persisted submission id to group by.
 */
export default function PopulationFileList({
  files,
  emptyText,
  auditId,
  controlId,
  canDelete = false,
  attributionLabel = "Submitted",
}: {
  files: PopulationFile[];
  emptyText: string;
  auditId?: number;
  controlId?: number;
  canDelete?: boolean;
  attributionLabel?: string;
}): JSX.Element {
  const authFetch = useAuthApiClient();
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const deleteFile = useDeletePopulationFile();
  const [deleteError, setDeleteError] = useState<string | null>(null);

  function handleDelete(fileId: number): void {
    if (auditId === undefined || controlId === undefined) return;
    setDeleteError(null);
    deleteFile.mutate(
      { auditId, controlId, fileId },
      { onError: (err) => setDeleteError(err instanceof Error ? err.message : "Failed to remove file") },
    );
  }

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

  if (files.length === 0) {
    return emptyText ? <Typography variant="body2" color="text.secondary">{emptyText}</Typography> : <></>;
  }

  const canRemove = canDelete && auditId !== undefined && controlId !== undefined;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      {(downloadError ?? deleteError) && (
        <Alert
          severity="error"
          onClose={() => { setDownloadError(null); setDeleteError(null); }}
          sx={{ fontSize: "0.8rem" }}
        >
          {downloadError ?? deleteError}
        </Alert>
      )}
      {files.map((f) => (
        <Box key={f.id} sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
          {(f.createdByName || f.createdBy) && (
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
              {attributionLabel} {formatTimestamp(f.createdAt)} · {f.createdByName || f.createdBy}
            </Typography>
          )}
          <Box
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
            {canRemove && (
              <IconButton
                size="small"
                aria-label={`Remove ${f.fileName}`}
                disabled={deleteFile.isPending}
                onClick={() => handleDelete(f.id)}
                sx={{ p: 0.5, color: "error.main", "&:hover": { bgcolor: "rgba(220,38,38,0.06)" } }}
              >
                {deleteFile.isPending && deleteFile.variables?.fileId === f.id
                  ? <CircularProgress size={13} color="inherit" />
                  : <Trash2 size={14} />}
              </IconButton>
            )}
          </Box>
        </Box>
      ))}
    </Box>
  );
}

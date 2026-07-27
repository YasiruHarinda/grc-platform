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
  Checkbox,
  Chip,
  Divider,
  FormControlLabel,
  Skeleton,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Lock, MessageSquare } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useGetEvidence } from "@modules/audit/api/useGetEvidence";
import { useAddComment, useGetComments } from "@modules/audit/api/useComments";
import { formatTimestamp } from "@modules/audit/utils/format";

/**
 * Comments on a control's latest evidence submission. Ticking "Internal only"
 * marks a comment is_internal, so the backend hides it from external auditors.
 */
export default function CommentsSection({
  auditId,
  controlId,
  canComment = true,
}: {
  auditId: number;
  controlId: number;
  canComment?: boolean;
}): JSX.Element {
  const evidence = useGetEvidence(auditId, controlId, true);
  // Comments attach to the latest evidence round (list is newest-first).
  const evidenceId = evidence.data?.[0]?.id ?? null;

  const comments = useGetComments(evidenceId);
  const addComment = useAddComment();

  const [text, setText] = useState("");
  const [internal, setInternal] = useState(false);

  function handleAdd() {
    if (evidenceId === null || text.trim() === "") return;
    addComment.mutate(
      { evidenceId, content: text.trim(), isInternal: internal },
      { onSuccess: () => { setText(""); setInternal(false); } },
    );
  }

  // No evidence submitted yet → nothing to comment on.
  if (evidence.isLoading) {
    return <Skeleton variant="rectangular" height={80} sx={{ borderRadius: 1 }} />;
  }
  if (evidenceId === null) {
    return (
      <Typography variant="body2" color="text.secondary">
        Comments become available once evidence is submitted for this control.
      </Typography>
    );
  }

  const items = comments.data ?? [];

  return (
    <Box>
      {comments.isLoading ? (
        <Skeleton variant="rectangular" height={56} sx={{ borderRadius: 1, mb: 2 }} />
      ) : comments.isError ? (
        <Typography variant="body2" color="error" sx={{ mb: 2 }}>Failed to load comments.</Typography>
      ) : items.length === 0 ? (
        <Box sx={{ py: 2, textAlign: "center", mb: 1 }}>
          <MessageSquare size={24} style={{ opacity: 0.2, margin: "0 auto 6px", display: "block" }} />
          <Typography variant="caption" color="text.secondary">No comments yet</Typography>
        </Box>
      ) : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1, mb: 2 }}>
          {items.map((c) => (
            <Box
              key={c.id}
              sx={(theme) => ({
                borderLeft: "3px solid",
                borderColor: c.isInternal
                  ? theme.palette.warning.main
                  : theme.palette.mode === "dark" ? "#1d4ed8" : "#93c5fd",
                pl: 2, py: 0.75,
                borderRadius: "0 4px 4px 0",
                bgcolor: c.isInternal
                  ? theme.palette.mode === "dark" ? "rgba(249,115,22,0.12)" : "rgba(234,88,12,0.06)"
                  : theme.palette.mode === "dark" ? "rgba(29,78,216,0.08)" : "#eff6ff",
              })}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
                  {c.createdBy || "Unknown"} · {formatTimestamp(c.createdAt)}
                </Typography>
                {c.isInternal && (
                  <Chip
                    size="small"
                    icon={<Lock size={11} />}
                    label="Internal"
                    sx={{ height: 18, fontSize: "0.65rem", "& .MuiChip-icon": { ml: 0.5 } }}
                    color="warning"
                    variant="outlined"
                  />
                )}
              </Box>
              <Typography variant="body2" sx={{ lineHeight: 1.7 }}>{c.content}</Typography>
            </Box>
          ))}
        </Box>
      )}

      {canComment && (
        <>
          <Divider sx={{ mb: 2 }} />

          {addComment.isError && (
            <Alert severity="error" sx={{ mb: 1.5, fontSize: "0.8rem" }}>
              {(addComment.error as Error).message}
            </Alert>
          )}

          <TextField
            fullWidth
            multiline
            minRows={3}
            placeholder="Add a comment…"
            value={text}
            onChange={(e) => setText(e.target.value)}
            size="small"
            sx={{ mb: 1 }}
          />
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1, flexWrap: "wrap" }}>
            <Tooltip title="Only compliance/internal staff will see this comment — hidden from the external auditor.">
              <FormControlLabel
                control={<Checkbox size="small" checked={internal} onChange={(e) => setInternal(e.target.checked)} />}
                label={<Typography variant="body2">Internal only</Typography>}
              />
            </Tooltip>
            <Button
              variant="contained"
              disableElevation
              disabled={text.trim().length === 0 || addComment.isPending}
              startIcon={<MessageSquare size={15} />}
              onClick={handleAdd}
              sx={{ textTransform: "none", fontWeight: 600 }}
            >
              {addComment.isPending ? "Posting…" : "Add Comment"}
            </Button>
          </Box>
        </>
      )}
    </Box>
  );
}

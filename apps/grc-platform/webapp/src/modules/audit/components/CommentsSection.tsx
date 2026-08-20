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
  CircularProgress,
  Divider,
  FormControlLabel,
  IconButton,
  Skeleton,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Lock, MessageSquare, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useAddComment, useDeleteComment, useGetComments } from "@modules/audit/api/useComments";
import { useCurrentUserEmail } from "@modules/audit/hooks/useCurrentUserEmail";
import { useAuditPrivileges } from "@modules/audit/hooks/useAuditPrivileges";
import { AuditPrivilege } from "@modules/audit/privileges";
import { formatTimestamp } from "@modules/audit/utils/format";

/**
 * One comment thread per control, spanning both the population and evidence
 * phases — available as soon as the control drawer is open, not gated on an
 * evidence/population round existing yet. Ticking "Internal only" marks a
 * comment is_internal, so the backend hides it from external auditors.
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
  const comments = useGetComments(auditId, controlId);
  const addComment = useAddComment();
  const deleteComment = useDeleteComment();
  const currentUserEmail = useCurrentUserEmail();
  const { can } = useAuditPrivileges();
  const isAdmin = can(AuditPrivilege.ManageControls);

  const [text, setText] = useState("");
  const [internal, setInternal] = useState(false);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  function handleAdd() {
    if (text.trim() === "") return;
    addComment.mutate(
      { auditId, controlId, content: text.trim(), isInternal: internal },
      { onSuccess: () => { setText(""); setInternal(false); } },
    );
  }

  function handleDelete(commentId: number) {
    setDeletingId(commentId);
    deleteComment.mutate(
      { auditId, controlId, commentId },
      { onSettled: () => setDeletingId(null) },
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
                // "rgba(59,130,246,0.6)" rather than a theme.palette.mode check:
                // this design system themes via CSS variables, so mode read
                // inside a plain sx callback doesn't reflect the active color
                // scheme (always resolves the light branch, even in dark mode —
                // see ControlDrawer.tsx's SampleSelectionCard for the same fix).
                borderColor: c.isInternal ? theme.palette.warning.main : "rgba(59,130,246,0.6)",
                pl: 2, py: 0.75,
                borderRadius: "0 4px 4px 0",
              })}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, flex: 1 }}>
                  {c.createdByName || c.createdBy || "Unknown"} · {formatTimestamp(c.createdAt)}
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
                {canComment && (isAdmin || c.createdBy === currentUserEmail) && (
                  <IconButton
                    size="small"
                    aria-label="Delete comment"
                    disabled={deletingId !== null}
                    onClick={() => handleDelete(c.id)}
                    sx={{ p: 0.5, color: "error.main", "&:hover": { bgcolor: "rgba(220,38,38,0.06)" } }}
                  >
                    {deletingId === c.id
                      ? <CircularProgress size={12} color="inherit" />
                      : <Trash2 size={13} />}
                  </IconButton>
                )}
              </Box>
              <Typography variant="body2" sx={{ lineHeight: 1.7 }}>{c.content}</Typography>
            </Box>
          ))}
        </Box>
      )}

      {deleteComment.isError && (
        <Alert severity="error" sx={{ mb: 1.5, fontSize: "0.8rem" }}>
          {(deleteComment.error as Error).message}
        </Alert>
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

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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { BACKEND_BASE_URL } from "@config/apiConfig";
import { extractErrorMessage } from "@modules/audit/api/apiError";

export interface AuditComment {
  id: number;
  controlId: number;
  parentCommentId: number | null;
  content: string;
  isInternal: boolean;
  createdBy: string;
  createdAt: string;
}

interface CommentListResponse {
  items: AuditComment[];
}

export const commentsQueryKey = (auditId: number, controlId: number) =>
  ["audit", "comments", auditId, controlId] as const;

/**
 * Lists comments for a control — one thread spanning both the population and
 * evidence phases, available as soon as the control drawer is open (internal
 * ones are hidden from external auditors by the backend).
 */
export function useGetComments(auditId: number, controlId: number) {
  const authFetch = useAuthApiClient();
  return useQuery({
    queryKey: commentsQueryKey(auditId, controlId),
    queryFn: async (): Promise<AuditComment[]> => {
      const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/comments`);
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to load comments (${res.status})`));
      }
      const body = (await res.json()) as CommentListResponse;
      return body.items ?? [];
    },
  });
}

interface AddCommentPayload {
  auditId: number;
  controlId: number;
  content: string;
  isInternal: boolean;
}

/** Posts a comment on a control. */
export function useAddComment() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ auditId, controlId, content, isInternal }: AddCommentPayload): Promise<AuditComment> => {
      const res = await authFetch(`${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/comments`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content, isInternal }),
      });
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to add comment (${res.status})`));
      }
      return res.json() as Promise<AuditComment>;
    },
    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: commentsQueryKey(auditId, controlId) });
    },
  });
}

interface DeleteCommentPayload {
  auditId: number;
  controlId: number;
  commentId: number;
}

/** Deletes a comment. The caller must be its author or hold ManageControls. */
export function useDeleteComment() {
  const authFetch = useAuthApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ auditId, controlId, commentId }: DeleteCommentPayload): Promise<void> => {
      const res = await authFetch(
        `${BACKEND_BASE_URL}/api/v1/audits/${auditId}/controls/${controlId}/comments/${commentId}`,
        { method: "DELETE" },
      );
      if (!res.ok) {
        throw new Error(await extractErrorMessage(res, `Failed to delete comment (${res.status})`));
      }
    },
    onSuccess: (_data, { auditId, controlId }) => {
      void queryClient.invalidateQueries({ queryKey: commentsQueryKey(auditId, controlId) });
    },
  });
}

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

import { Box, Button, Typography } from "@wso2/oxygen-ui";
import type { JSX, ReactNode } from "react";

interface EmptyStateProps {
  /** Optional illustration or icon shown above the title. */
  icon?: ReactNode;
  /** Main heading, e.g. "No controls yet". */
  title: string;
  /** Optional supporting line below the title. */
  message?: string;
  /** Optional call-to-action button label. Requires onAction to render. */
  actionLabel?: string;
  /** Click handler for the action button. */
  onAction?: () => void;
}

/**
 * Shared placeholder shown when a list/table has no data. Props-driven so both
 * the Audit and Risk modules reuse it with their own text/icon/action.
 *
 * @param {EmptyStateProps} props - Content for the empty state.
 * @returns {JSX.Element} The empty state.
 */
export default function EmptyState({
  icon,
  title,
  message,
  actionLabel,
  onAction,
}: EmptyStateProps): JSX.Element {
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        textAlign: "center",
        gap: 1.5,
        py: 6,
        px: 3,
      }}
    >
      {icon && <Box sx={{ color: "text.secondary", mb: 1 }}>{icon}</Box>}
      <Typography variant="h6">{title}</Typography>
      {message && (
        <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 420 }}>
          {message}
        </Typography>
      )}
      {actionLabel && onAction && (
        <Button variant="contained" onClick={onAction} sx={{ mt: 1 }}>
          {actionLabel}
        </Button>
      )}
    </Box>
  );
}

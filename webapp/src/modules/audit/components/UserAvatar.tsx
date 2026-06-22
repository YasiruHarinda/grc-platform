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

import { Avatar, Tooltip } from "@mui/material";
import type { JSX } from "react";

// Consistent palette — each name always gets the same color
const PALETTE = [
  "#64b5f6",
  "#81c784",
  "#ffb74d",
  "#ce93d8",
  "#4dd0e1",
  "#ef9a9a",
  "#9fa8da",
  "#80cbc4",
  "#f48fb1",
  "#fff176",
];

function nameToColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  return PALETTE[Math.abs(hash) % PALETTE.length];
}

function getInitials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length >= 2) {
    return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

export interface UserAvatarProps {
  name: string;
  /** Profile photo URL from Asgardeo. Falls back to initials if not provided. */
  src?: string | null;
  /** Avatar diameter in px. Defaults to 28. */
  size?: number;
}

export default function UserAvatar({ name, src, size = 28 }: UserAvatarProps): JSX.Element {
  return (
    <Tooltip title={name} arrow>
      <Avatar
        src={src ?? undefined}
        alt={name}
        sx={{
          width: size,
          height: size,
          fontSize: size * 0.38,
          fontWeight: 600,
          bgcolor: src ? undefined : nameToColor(name),
          flexShrink: 0,
        }}
      >
        {!src && getInitials(name)}
      </Avatar>
    </Tooltip>
  );
}

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

import { Chip, Menu, MenuItem } from "@wso2/oxygen-ui";
import { type JSX, useState } from "react";
import { CONTROL_STATUS_COLORS, CONTROL_STATUS_LABELS, overridableStatuses } from "@modules/audit/utils/controlStatus";
import type { ControlStatus } from "@modules/audit/types/audit";

interface ControlStatusChipProps {
  status: ControlStatus;
  size?: "small" | "medium";
  // editable=true renders the chip as an override trigger (drawer, admin-only —
  // see ControlDrawer). Picking a status does not commit it: onOverride is
  // called with the chosen target so the caller can confirm before writing.
  editable?: boolean;
  requirementType?: "DESIGN" | "OE";
  onOverride?: (target: ControlStatus) => void;
}

export default function ControlStatusChip({
  status,
  size = "small",
  editable = false,
  requirementType = "OE",
  onOverride,
}: ControlStatusChipProps): JSX.Element {
  const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);
  const color = CONTROL_STATUS_COLORS[status];
  const options = editable ? overridableStatuses(status, requirementType) : [];

  const chip = (
    <Chip
      label={CONTROL_STATUS_LABELS[status]}
      size={size}
      variant="outlined"
      onClick={editable && options.length > 0 ? (e) => setMenuAnchor(e.currentTarget) : undefined}
      sx={{
        color,
        borderColor: color,
        bgcolor: "transparent",
        fontWeight: 500,
        cursor: editable && options.length > 0 ? "pointer" : undefined,
        "& .MuiChip-label": { px: 1.25 },
      }}
    />
  );

  if (!editable || options.length === 0) {
    return chip;
  }

  return (
    <>
      {chip}
      <Menu anchorEl={menuAnchor} open={Boolean(menuAnchor)} onClose={() => setMenuAnchor(null)}>
        {options.map((target) => (
          <MenuItem
            key={target}
            onClick={() => {
              setMenuAnchor(null);
              onOverride?.(target);
            }}
          >
            {CONTROL_STATUS_LABELS[target]}
          </MenuItem>
        ))}
      </Menu>
    </>
  );
}

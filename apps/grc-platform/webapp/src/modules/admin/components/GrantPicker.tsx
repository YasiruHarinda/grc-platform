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
  Box,
  Button,
  FormControl,
  InputLabel,
  ListSubheader,
  MenuItem,
  Select,
  Stack,
  type SxProps,
  type Theme,
  Typography,
} from "@wso2/oxygen-ui";
import { type JSX, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { fetchAuditScopeTeams, fetchScopeTeams, type Role, type ScopeTeam } from "../api/adminApi";

export interface PendingGrant {
  roleId: number;
  scopeType: "GLOBAL" | "RISK_TEAM" | "AUDIT_TEAM";
  scopeId: number;
}

interface GrantPickerProps {
  roles: Role[];
  onAdd: (grant: PendingGrant, label: string) => void;
  // userType narrows the Role dropdown to roles assignable to that kind of
  // person (Role.assignableUserType). The rule is asymmetric, mirroring the
  // backend (validateUserType): an External person is offered only
  // EXTERNAL-assignable roles (currently just external-auditor), while an
  // Internal person is offered everything, including external-auditor.
  // Omit while the person's type isn't chosen yet; the backend enforces the
  // same rule regardless of what this offers.
  userType?: "INTERNAL" | "EXTERNAL";
}

// Centres the resting label; the theme's top:-7px needs !important to undo.
const centredLabelSx: SxProps<Theme> = {
  "& .MuiInputLabel-outlined:not(.MuiInputLabel-shrink)": {
    top: "0 !important",
    height: "100%",
    display: "flex",
    alignItems: "center",
    transform: "translate(14px, 0) scale(1)",
  },
};

const hubLabel: Record<Role["module"], string> = {
  RISK: "Risk Hub",
  AUDIT: "Audit Hub",
  SHARED: "Platform-wide",
};

// The Role + Scope + Add row shared by AddUserDialog's stage 2 and
// GrantEditorDialog. Role is one flat dropdown grouped by hub (section
// headers, not a separate hub-picking step) — the scope picker's shape then
// depends on the selected role's module/scopeBasis:
//   - SHARED role                 → Scope is always Global, not editable.
//   - RISK role, SOURCE_REGISTER  → Global, or a source-register team.
//   - RISK role, ASSIGNMENT_TEAM  → Global, or an assignment team.
//   - AUDIT role                  → Global, or an audit team (unless it's the
//                                   one EXTERNAL-only role, which locks to
//                                   Global the same way SHARED does — its
//                                   real scoping is per-control assignment,
//                                   not team-based).
export default function GrantPicker({ roles, onAdd, userType }: GrantPickerProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const [roleId, setRoleId] = useState<number | "">("");
  const [scopeId, setScopeId] = useState<number | "GLOBAL">("GLOBAL");
  const [teams, setTeams] = useState<ScopeTeam[]>([]);
  const [teamsLoading, setTeamsLoading] = useState(false);

  const offeredRoles = userType === "EXTERNAL" ? roles.filter((r) => r.assignableUserType === "EXTERNAL") : roles;
  const selectedRole = offeredRoles.find((r) => r.id === roleId);

  // Reset a selection that's no longer offered (e.g. the person's type just
  // changed) rather than leaving a stale, now-invalid role selected.
  useEffect(() => {
    if (roleId !== "" && !offeredRoles.some((r) => r.id === roleId)) {
      setRoleId("");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [userType]);

  useEffect(() => {
    setScopeId("GLOBAL");
    if (!selectedRole || selectedRole.assignableUserType === "EXTERNAL") {
      setTeams([]);
      setTeamsLoading(false);
      return;
    }
    if (selectedRole.module === "AUDIT") {
      let cancelled = false;
      setTeamsLoading(true);
      fetchAuditScopeTeams(authFetch)
        .then((t) => {
          if (!cancelled) setTeams(t);
        })
        .catch(() => {
          if (!cancelled) setTeams([]);
        })
        .finally(() => {
          if (!cancelled) setTeamsLoading(false);
        });
      return () => {
        cancelled = true;
      };
    }
    if (selectedRole.module !== "RISK" || !selectedRole.scopeBasis) {
      setTeams([]);
      setTeamsLoading(false);
      return;
    }
    // Guards against two races when the role selection changes quickly: a
    // stale response overwriting the newer role's teams if requests resolve
    // out of order, and teamsLoading getting stuck true if a later role
    // switch takes an early-return branch above before this one settles.
    let cancelled = false;
    const type = selectedRole.scopeBasis === "SOURCE_REGISTER" ? "SOURCE_REGISTER" : "ASSIGNMENT";
    setTeamsLoading(true);
    fetchScopeTeams(authFetch, type)
      .then((t) => {
        if (!cancelled) setTeams(t);
      })
      .catch(() => {
        if (!cancelled) setTeams([]);
      })
      .finally(() => {
        if (!cancelled) setTeamsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roleId]);

  const handleAdd = () => {
    if (!selectedRole || roleId === "") return;
    const scopeType: PendingGrant["scopeType"] =
      scopeId === "GLOBAL" ? "GLOBAL" : selectedRole.module === "AUDIT" ? "AUDIT_TEAM" : "RISK_TEAM";
    const scopeName =
      scopeId === "GLOBAL" ? "Global (ALL)" : teams.find((t) => t.id === scopeId)?.name ?? "Global (ALL)";
    onAdd(
      { roleId: selectedRole.id, scopeType, scopeId: scopeId === "GLOBAL" ? 0 : scopeId },
      `${selectedRole.roleName} @ ${scopeName}`,
    );
  };

  const scopeLocked = !selectedRole || selectedRole.module === "SHARED" || selectedRole.assignableUserType === "EXTERNAL";
  const scopeHint = scopeLocked
    ? selectedRole?.assignableUserType === "EXTERNAL"
      ? "This role is scoped to the controls assigned to this person, not to a team."
      : "This role always applies platform-wide."
    : selectedRole?.module === "AUDIT"
      ? "Global (ALL) applies to every team, or pick one audit team to limit it to."
      : selectedRole?.scopeBasis === "SOURCE_REGISTER"
        ? "Global (ALL) applies to every register, or pick one register to limit it to."
        : "Global (ALL) applies to every team, or pick one team to limit it to.";

  // Grouped by hub via section headers rather than a separate hub-first step —
  // one dropdown stays a smaller change on top of what already existed here,
  // and still tells Risk/Audit/Platform-wide roles apart at a glance.
  const grouped = (["RISK", "AUDIT", "SHARED"] as const)
    .map((module) => ({ module, roles: offeredRoles.filter((r) => r.module === module) }))
    .filter((g) => g.roles.length > 0);

  return (
    <Box sx={{ mt: 1.5 }}>
      <Stack direction="row" spacing={1.5} alignItems="flex-start">
        <Box sx={{ flex: 1, minWidth: 160 }}>
          <FormControl size="small" fullWidth sx={centredLabelSx}>
            <InputLabel id="grant-role-label">Role</InputLabel>
            <Select
              labelId="grant-role-label"
              label="Role"
              value={roleId}
              onChange={(e) => setRoleId(Number(e.target.value))}
            >
              {grouped.flatMap((g) => [
                <ListSubheader key={`h-${g.module}`}>{hubLabel[g.module]}</ListSubheader>,
                ...g.roles.map((r) => (
                  <MenuItem key={r.id} value={r.id}>
                    {r.roleName}
                  </MenuItem>
                )),
              ])}
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5, ml: 0.5 }}>
            What this person is allowed to do.
          </Typography>
        </Box>

        <Box sx={{ flex: 1, minWidth: 160 }}>
          <FormControl size="small" fullWidth sx={centredLabelSx}>
            <InputLabel id="grant-scope-label">Scope</InputLabel>
            {scopeLocked ? (
              <Select labelId="grant-scope-label" label="Scope" value="GLOBAL" disabled>
                <MenuItem value="GLOBAL">Global (ALL)</MenuItem>
              </Select>
            ) : (
              <Select
                labelId="grant-scope-label"
                label="Scope"
                value={scopeId}
                disabled={teamsLoading}
                onChange={(e) => setScopeId(e.target.value === "GLOBAL" ? "GLOBAL" : Number(e.target.value))}
              >
                <MenuItem value="GLOBAL">Global (ALL)</MenuItem>
                {teams.map((t) => (
                  <MenuItem key={t.id} value={t.id}>
                    {t.name}
                  </MenuItem>
                ))}
              </Select>
            )}
          </FormControl>
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5, ml: 0.5 }}>
            {scopeHint}
          </Typography>
        </Box>

        <Button variant="contained" onClick={handleAdd} disabled={roleId === ""} sx={{ mt: 0.25 }}>
          Add
        </Button>
      </Stack>
    </Box>
  );
}

// Empty-state note shown above the picker when a role list failed to load —
// kept here so both call sites render it identically.
export function NoRolesNote(): JSX.Element {
  return (
    <Box sx={{ mt: 1 }}>
      <Typography variant="body2" color="text.secondary" fontStyle="italic">
        No roles available to grant.
      </Typography>
    </Box>
  );
}

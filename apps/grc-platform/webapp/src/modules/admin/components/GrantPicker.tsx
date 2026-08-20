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

import { Box, Button, FormControl, InputLabel, MenuItem, Select, Stack, Typography } from "@wso2/oxygen-ui";
import { type JSX, useEffect, useState } from "react";
import { useAuthApiClient } from "@hooks/useAuthApiClient";
import { fetchScopeTeams, type Role, type ScopeTeam } from "../api/adminApi";

export interface PendingGrant {
  roleId: number;
  scopeType: "GLOBAL" | "RISK_TEAM" | "AUDIT_TEAM";
  scopeId: number;
}

interface GrantPickerProps {
  roles: Role[];
  onAdd: (grant: PendingGrant, label: string) => void;
}

// The Role + Scope + Add row shared by AddUserDialog's stage 2 and
// GrantEditorDialog — the scope picker's shape depends on the selected
// role's module/scopeBasis (see the mockup's scopeFieldHTML for the
// reference behaviour this reimplements as real component state):
//   - SHARED role            → Scope is always Global, not editable.
//   - RISK role, SOURCE_REGISTER  → Global, or a source-register team.
//   - RISK role, ASSIGNMENT_TEAM  → Global, or an assignment team.
export default function GrantPicker({ roles, onAdd }: GrantPickerProps): JSX.Element {
  const authFetch = useAuthApiClient();
  const [roleId, setRoleId] = useState<number | "">("");
  const [scopeId, setScopeId] = useState<number | "GLOBAL">("GLOBAL");
  const [teams, setTeams] = useState<ScopeTeam[]>([]);
  const [teamsLoading, setTeamsLoading] = useState(false);

  const selectedRole = roles.find((r) => r.id === roleId);

  useEffect(() => {
    setScopeId("GLOBAL");
    if (!selectedRole || selectedRole.module !== "RISK" || !selectedRole.scopeBasis) {
      setTeams([]);
      return;
    }
    const type = selectedRole.scopeBasis === "SOURCE_REGISTER" ? "SOURCE_REGISTER" : "ASSIGNMENT";
    setTeamsLoading(true);
    fetchScopeTeams(authFetch, type)
      .then(setTeams)
      .catch(() => setTeams([]))
      .finally(() => setTeamsLoading(false));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roleId]);

  const handleAdd = () => {
    if (!selectedRole || roleId === "") return;
    const scopeType: PendingGrant["scopeType"] = scopeId === "GLOBAL" ? "GLOBAL" : "RISK_TEAM";
    const scopeName = scopeId === "GLOBAL" ? "Global" : teams.find((t) => t.id === scopeId)?.name ?? "Global";
    onAdd(
      { roleId: selectedRole.id, scopeType, scopeId: scopeId === "GLOBAL" ? 0 : scopeId },
      `${selectedRole.roleName} @ ${scopeName}`,
    );
  };

  const scopeLocked = !selectedRole || selectedRole.module === "SHARED";

  return (
    <Stack direction="row" spacing={1.5} alignItems="flex-end" sx={{ mt: 1.5 }}>
      <FormControl size="small" sx={{ flex: 1, minWidth: 160 }}>
        <InputLabel id="grant-role-label">Role</InputLabel>
        <Select
          labelId="grant-role-label"
          label="Role"
          value={roleId}
          onChange={(e) => setRoleId(Number(e.target.value))}
        >
          {roles.map((r) => (
            <MenuItem key={r.id} value={r.id}>
              {r.roleName}
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <FormControl size="small" sx={{ flex: 1, minWidth: 160 }}>
        <InputLabel id="grant-scope-label">Scope</InputLabel>
        {scopeLocked ? (
          <Select labelId="grant-scope-label" label="Scope" value="GLOBAL" disabled>
            <MenuItem value="GLOBAL">Global</MenuItem>
          </Select>
        ) : (
          <Select
            labelId="grant-scope-label"
            label="Scope"
            value={scopeId}
            disabled={teamsLoading}
            onChange={(e) => setScopeId(e.target.value === "GLOBAL" ? "GLOBAL" : Number(e.target.value))}
          >
            <MenuItem value="GLOBAL">Global</MenuItem>
            {teams.map((t) => (
              <MenuItem key={t.id} value={t.id}>
                {t.name}
              </MenuItem>
            ))}
          </Select>
        )}
      </FormControl>

      <Button variant="contained" onClick={handleAdd} disabled={roleId === ""}>
        Add
      </Button>
    </Stack>
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

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

import { FormControl, InputLabel, MenuItem, Select } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { RiskTeam } from "../../api/riskApi";

interface RegisterFilterProps {
  teams: RiskTeam[];
  value: number; // 0 = All Registers
  onChange: (registerId: number) => void;
}

// Page-level register filter; scopes every chart on the Analytics page.
// The register-comparison donut only renders when value === 0 ("All").
export default function RegisterFilter({ teams, value, onChange }: RegisterFilterProps): JSX.Element {
  return (
    <FormControl size="small" sx={{ minWidth: 200 }}>
      <InputLabel>Register</InputLabel>
      <Select
        label="Register"
        value={value || ""}
        onChange={(e) => onChange(Number(e.target.value) || 0)}
      >
        <MenuItem value="">All Registers</MenuItem>
        {teams.map((t) => (
          <MenuItem key={t.id} value={t.id}>
            {t.name}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
}

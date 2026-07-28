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

// Page-level register filter; scopes every chart on the Analytics and Dashboard
// pages. The register-comparison donut only renders when value === 0 ("All").
//
// No size="small" on the FormControl. Oxygen UI's theme already defaults Select
// to small and nudges the resting label up 7px to re-centre it against a
// normal-height FormControl; making the FormControl small as well applies that
// correction twice and the label sits above centre.
//
// A plain outlined Select, matching the filters on the Risk Registers page.
// It was previously wrapped in a Paper card whose border sat outside the
// Select's own outline, so the control drew two borders and stood taller than
// the filters elsewhere. The inner outline was suppressed with a raw MUI class
// selector, which both stopped working across an Oxygen UI upgrade and removed
// the notch the floating label needs to sit in.
export default function RegisterFilter({ teams, value, onChange }: RegisterFilterProps): JSX.Element {
  return (
    <FormControl sx={{ minWidth: 200 }}>
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

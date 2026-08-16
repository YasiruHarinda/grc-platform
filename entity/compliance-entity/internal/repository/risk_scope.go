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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import "strings"

// scopeFilter returns an optional " AND (...)" clause and its bind args,
// restricting rows to risks the caller may see.
//
// The two lists match DIFFERENT columns and are ORed:
//
//	sourceIDs     → r.source_register_id  (risks raised in these registers)
//	assignmentIDs → r.assignment_team_id  (work routed to these teams)
//
// They are separate because different roles are about different dimensions of
// a risk — a Risk Owner of HR should see work routed to HR, not risks HR
// happened to raise. A single list applied to both columns conflated the two,
// which mattered because a risk_team row can be both a register and an
// assignment team.
//
// Both empty means unrestricted. Classifying whether a caller needs scoping at
// all happens in the GRC backend, which resolves their grants and only then
// populates these — this package trusts what it is given rather than
// re-deriving a role here.
//
// alias is the risk table's alias in the surrounding query ("r" for the main
// FROM risk, "r2" for a correlated subquery's copy).
func scopeFilter(alias string, sourceIDs, assignmentIDs []int) (string, []any) {
	if len(sourceIDs) == 0 && len(assignmentIDs) == 0 {
		return "", nil
	}
	var parts []string
	var args []any
	if len(sourceIDs) > 0 {
		parts = append(parts, alias+".source_register_id IN ("+placeholders(len(sourceIDs))+")")
		for _, id := range sourceIDs {
			args = append(args, id)
		}
	}
	if len(assignmentIDs) > 0 {
		parts = append(parts, alias+".assignment_team_id IN ("+placeholders(len(assignmentIDs))+")")
		for _, id := range assignmentIDs {
			args = append(args, id)
		}
	}
	return " AND (" + strings.Join(parts, " OR ") + ")", args
}

// registerScopeFilter restricts to risks RAISED in the given registers.
//
// Used by the dashboard and analytics, which are rendered per register and so
// aggregate by source register alone: a risk raised in Choreo and routed to
// Asgardeo belongs to Choreo's numbers, however visible it is to Asgardeo's
// people in the registers list.
func registerScopeFilter(alias string, registerIDs []int) (string, []any) {
	if len(registerIDs) == 0 {
		return "", nil
	}
	args := make([]any, 0, len(registerIDs))
	for _, id := range registerIDs {
		args = append(args, id)
	}
	return " AND " + alias + ".source_register_id IN (" + placeholders(len(registerIDs)) + ")", args
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

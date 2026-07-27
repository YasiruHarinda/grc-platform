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

// teamScopeFilter returns an optional " AND (...)" clause and its bind args,
// restricting rows to risks whose source register or assignment team is one
// of teamIDs — how Risk Assigner/Risk Owner-only callers are scoped to their
// own risk teams (a risk is visible if EITHER field matches, since a risk
// raised in one team is often assigned to another).
//
// Empty teamIDs means unrestricted. Classifying whether a caller needs scoping
// at all (versus seeing everything, e.g. Compliance/Management/Admin) happens
// in the GRC backend, which resolves the caller's privileges and team
// memberships and only then populates this field — this package trusts what
// it's given rather than re-deriving a role here.
//
// alias is the risk table's alias in the surrounding query ("r" for the main
// FROM risk, "r2" for a correlated subquery's copy).
func teamScopeFilter(alias string, teamIDs []int) (string, []any) {
	if len(teamIDs) == 0 {
		return "", nil
	}
	ph := strings.Repeat("?,", len(teamIDs))
	ph = ph[:len(ph)-1]
	args := make([]any, 0, len(teamIDs)*2)
	for _, id := range teamIDs {
		args = append(args, id)
	}
	for _, id := range teamIDs {
		args = append(args, id)
	}
	return " AND (" + alias + ".source_register_id IN (" + ph + ") OR " + alias + ".assignment_team_id IN (" + ph + "))", args
}

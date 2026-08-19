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

// Package user defines the shared User entity referenced by both the Risk and
// Audit modules. The `user` table is defined in shared.sql and is shared across
// both modules.
package user

// User maps to the shared `user` table, which is owned by the Compliance
// Entity — this struct mirrors the subset of its /users payload the GRC
// backend needs. RiskTeamIDs is a user's risk-team memberships (zero or more)
// via the entity's user_risk_team join table — empty, never nil, when a user
// belongs to no risk team.
type User struct {
	ID int `json:"id"`
	// UUID is the user's Asgardeo id — the same value their token carries as
	// `sub`, and this platform's identity for them. Empty for a row that
	// predates the identity migration and whose email matched no Asgardeo
	// account; such a user cannot be resolved by uuid.
	UUID string `json:"uuid"`
	// DisplayName and Email are being removed from the database — a security
	// review required that the platform stop storing them. Both are still
	// populated for as long as the columns exist; new code should resolve a
	// person's name from the identity directory by UUID instead.
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Status      string `json:"status"` // ACTIVE | INACTIVE | REMOVED
	RiskTeamIDs []int  `json:"risk_team_ids"`
}

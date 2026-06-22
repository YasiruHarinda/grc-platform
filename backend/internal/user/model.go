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

// User maps to the shared `user` table in risk_schema.sql / audit_schema.sql.
// TODO: add fields based on the user table definition
type User struct {
	ID          int    `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Status      string `json:"status"` // ACTIVE | INACTIVE | REMOVED
}

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

package model

// Team represents a risk team, mapping to the `risk_team` table.
type Team struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
	TeamType    string  `json:"team_type"`
	Status      string  `json:"status"`
}

// ListTeamsFilter controls which teams are returned by GET /api/v1/teams.
// Type uses semantic values: "SOURCE_REGISTER" returns teams where team_type
// IN ('SOURCE_REGISTER','BOTH'); "ASSIGNMENT" returns IN ('ASSIGNMENT','BOTH').
// Empty Type returns all ACTIVE teams.
type ListTeamsFilter struct {
	Type string
}

// CreateTeamRequest is the payload for POST /api/v1/teams.
type CreateTeamRequest struct {
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description string  `json:"description"`
	TeamType    string  `json:"team_type"`
}

// UpdateTeamRequest is the payload for PUT /api/v1/teams/{id}.
type UpdateTeamRequest struct {
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
}

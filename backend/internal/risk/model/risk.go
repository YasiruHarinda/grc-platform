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

// Package model defines the domain types for the Risk Hub module.
package model

// Risk represents a GRC risk item.
// TODO: add fields based on the `risk` table in risk_schema.sql
type Risk struct{}

// ListRisksFilter holds query parameters for filtering the risk list.
// TODO: define fields (status, register, level, search, teamID, etc.)
type ListRisksFilter struct{}

// CreateRiskRequest is the payload for POST /api/v1/risks.
// TODO: define required and optional fields
type CreateRiskRequest struct{}

// UpdateRiskRequest is the payload for PUT /api/v1/risks/{id}.
// TODO: define updatable fields
type UpdateRiskRequest struct{}

// RejectRiskRequest carries the mandatory rejection comment.
type RejectRiskRequest struct {
	RejectionComment string `json:"rejection_comment"`
}

// EscalateRiskRequest carries escalation details.
// TODO: define fields
type EscalateRiskRequest struct{}

// AssessRiskRequest carries residual assessment values.
// TODO: define fields
type AssessRiskRequest struct{}

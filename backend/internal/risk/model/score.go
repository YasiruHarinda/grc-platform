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

// RiskScore represents a likelihood/impact scoring configuration.
// TODO: add fields based on the `risk_score` table in risk_schema.sql
type RiskScore struct{}

// CreateRiskScoreRequest is the payload for POST /api/v1/risk-scores.
// TODO: define fields
type CreateRiskScoreRequest struct{}

// UpdateRiskScoreRequest is the payload for PUT /api/v1/risk-scores/{id}.
// TODO: define fields
type UpdateRiskScoreRequest struct{}

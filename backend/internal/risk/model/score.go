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

// RiskScore represents one cell of the 3×3 likelihood × impact matrix,
// mapping to the `risk_score` table.
type RiskScore struct {
	ID         int    `json:"id"`
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskRating int    `json:"risk_rating"`
	RiskLevel  string `json:"risk_level"`
	ColorCode  string `json:"color_code"`
}

// CreateRiskScoreRequest is the payload for POST /api/v1/risk-scores.
type CreateRiskScoreRequest struct {
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskRating int    `json:"risk_rating"`
	RiskLevel  string `json:"risk_level"`
	ColorCode  string `json:"color_code"`
}

// UpdateRiskScoreRequest is the payload for PUT /api/v1/risk-scores/{id}.
type UpdateRiskScoreRequest struct {
	RiskLevel string `json:"risk_level"`
	ColorCode string `json:"color_code"`
}

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

import "time"

// RiskAssessment represents one residual risk reassessment entry,
// mapping to the `risk_assessment` table.
type RiskAssessment struct {
	ID               int       `json:"id"`
	RiskID           int       `json:"risk_id"`
	ScoreID          int       `json:"score_id"`
	Progress         string    `json:"progress"`
	ReassessmentDate string    `json:"reassessment_date"`
	AssessedBy       string    `json:"assessed_by"`
	CreatedAt        time.Time `json:"created_at"`
	// Resolved from risk_score join — populated by GetByID/List queries.
	ResidualLikelihood int    `json:"residual_likelihood"`
	ResidualImpact     int    `json:"residual_impact"`
	ResidualRating     int    `json:"residual_rating"`
	ResidualLevel      string `json:"residual_level"`
	ResidualColorCode  string `json:"residual_color_code"`
	// IsInitial marks a synthetic entry representing the risk's gross score,
	// prepended by GetByID so the assessment log always shows the full lineage
	// (gross → reassessment → reassessment ...) even though it isn't a real
	// risk_assessment row.
	IsInitial bool `json:"is_initial,omitempty"`
}

// CreateAssessmentRequest is the payload for POST /api/v1/risks/{id}/assess.
type CreateAssessmentRequest struct {
	Likelihood       int    `json:"likelihood"`
	Impact           int    `json:"impact"`
	Progress         string `json:"progress"`
	ReassessmentDate string `json:"reassessment_date"`
}

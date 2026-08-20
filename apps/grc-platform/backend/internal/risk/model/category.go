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

// RiskCategory represents a fixed-but-extensible risk classification (e.g.
// "PII / Sensitive Data Exposure"), mapping to the `risk_category` table.
type RiskCategory struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// CreateRiskCategoryRequest is the payload for POST /api/v1/risk-categories.
type CreateRiskCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateRiskCategoryRequest is the payload for PUT /api/v1/risk-categories/{id}.
type UpdateRiskCategoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

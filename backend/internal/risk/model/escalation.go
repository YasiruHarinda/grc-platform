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

// Escalation represents a risk escalation record, mapping to `risk_escalation`.
type Escalation struct {
	ID                   int     `json:"id"`
	RiskID               int     `json:"risk_id"`
	EscalatedTo          int     `json:"escalated_to"`
	Reason               *string `json:"reason"`
	NewTreatmentStrategy *string `json:"new_treatment_strategy"`
	ActionPlanID         *int    `json:"action_plan_id"`
	Decision             *string `json:"decision"`
	Status               string  `json:"status"`
}

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

// ActionPlan represents a remediation plan attached to a risk,
// mapping to the `risk_action_plan` table.
type ActionPlan struct {
	ID            int     `json:"id"`
	RiskID        int     `json:"risk_id"`
	ActionOwnerID *int    `json:"action_owner_id"`
	Description   *string `json:"description"`
	Status        string  `json:"status"`
	CompletedDate *string `json:"completed_date"`
	PlanType      string  `json:"plan_type"`
}

// ActionPlanStep represents an individual step within an action plan,
// mapping to the `risk_action_step` table.
type ActionPlanStep struct {
	ID            int     `json:"id"`
	PlanID        int     `json:"plan_id"`
	StepNo        int     `json:"step_no"`
	Description   *string `json:"description"`
	Status        string  `json:"status"`
	CompletedDate *string `json:"completed_date"`
}

// CreateActionPlanRequest is the payload for POST /api/v1/risks/{id}/action-plans.
type CreateActionPlanRequest struct {
	Description   string `json:"description"`
	ActionOwnerID *int   `json:"action_owner_id"`
	PlanType      string `json:"plan_type"`
}

// UpdateActionPlanRequest is the payload for PUT /api/v1/risks/{id}/action-plans/{planId}.
type UpdateActionPlanRequest struct {
	Description   string  `json:"description"`
	Status        string  `json:"status"`
	CompletedDate *string `json:"completed_date"`
}

// AddActionPlanStepRequest is the payload for POST .../steps.
type AddActionPlanStepRequest struct {
	Description string `json:"description"`
}

// UpdateActionPlanStepRequest is the payload for PUT .../steps/{stepId}.
type UpdateActionPlanStepRequest struct {
	Description   string  `json:"description"`
	Status        string  `json:"status"`
	CompletedDate *string `json:"completed_date"`
}

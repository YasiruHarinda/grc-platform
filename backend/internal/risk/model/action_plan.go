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

// ActionPlan represents a remediation plan attached to a risk.
// TODO: add fields based on `action_plan` table in risk_schema.sql
type ActionPlan struct{}

// ActionPlanStep represents an individual step within an action plan.
// TODO: add fields based on `action_plan_step` table
type ActionPlanStep struct{}

// CreateActionPlanRequest is the payload for POST /api/v1/risks/{id}/action-plans.
// TODO: define fields
type CreateActionPlanRequest struct{}

// UpdateActionPlanRequest is the payload for PUT /api/v1/risks/{id}/action-plans/{planId}.
// TODO: define fields
type UpdateActionPlanRequest struct{}

// AddActionPlanStepRequest is the payload for POST .../steps.
// TODO: define fields (description, assigned_to, due_date, etc.)
type AddActionPlanStepRequest struct{}

// UpdateActionPlanStepRequest is the payload for PUT .../steps/{stepId}.
// TODO: define fields
type UpdateActionPlanStepRequest struct{}

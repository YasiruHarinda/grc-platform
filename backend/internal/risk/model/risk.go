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

import "time"

// Risk represents a GRC risk item, mapping to the `risk` table.
type Risk struct {
	ID                     int        `json:"id"`
	RiskYear               int        `json:"risk_year"`
	SourceRegisterID       int        `json:"source_register_id"`
	RiskQuarter            string     `json:"risk_quarter"`
	RiskCode               string     `json:"risk_code"`
	RiskTitle              string     `json:"risk_title"`
	RiskDescription        string     `json:"risk_description"`
	RiskIdentifiedDate     *string    `json:"risk_identified_date"`
	IdentifiedByType       *string    `json:"identified_by_type"`
	IdentifiedByUserID     *int       `json:"identified_by_user_id"`
	IdentifiedByName       *string    `json:"identified_by_name"`
	AssignerID             int        `json:"assigner_id"`
	OwnerID                int        `json:"owner_id"`
	ImpactDescription      *string    `json:"impact_description"`
	GrossScoreID           *int       `json:"gross_score_id"`
	TreatmentStrategy      *string    `json:"treatment_strategy"`
	ActionPlanID           *int       `json:"action_plan_id"`
	AssignmentTeamID       int        `json:"assignment_team_id"`
	Progress               *string    `json:"progress"`
	ImplementationDate     *string    `json:"implementation_date"`
	ReassessmentDate       *string    `json:"reassessment_date"`
	ComplianceApprovalBy   *int       `json:"compliance_approval_by"`
	ComplianceApprovalDate *string    `json:"compliance_approval_date"`
	GitIssueURL            *string    `json:"git_issue_url"`
	EmailSubject           *string    `json:"email_subject"`
	Remarks                *string    `json:"remarks"`
	WorkflowStatus         string     `json:"workflow_status"`
	RejectionComment       *string    `json:"rejection_comment"`
	CreatedAt              time.Time  `json:"created_at"`
	CreatedBy              string     `json:"created_by"`
	UpdatedAt              time.Time  `json:"updated_at"`
	UpdatedBy              string     `json:"updated_by"`
}

// CreateRiskRequest is the payload for POST /api/v1/risks.
// All FK references use integer IDs resolved from the frontend's lookup lists.
// Dates are YYYY-MM-DD strings. Evidence files are uploaded separately after creation.
type CreateRiskRequest struct {
	// Step 1: Basic Information
	Year                   int     `json:"year"`
	Quarter                string  `json:"quarter"`
	SourceRegisterID       int     `json:"source_register_id"`
	RiskTitle              string  `json:"risk_title"`
	RiskDescription        string  `json:"risk_description"`
	ComplianceReferenceIDs []int   `json:"compliance_reference_ids"`
	IdentifiedByType       string  `json:"identified_by_type"`
	IdentifiedByUserID     *int    `json:"identified_by_user_id,omitempty"`
	IdentifiedByName       *string `json:"identified_by_name,omitempty"`
	AssignerID             int     `json:"assigner_id"`
	RiskIdentifiedDate     string  `json:"risk_identified_date"`

	// Step 2: Risk Assessment
	Likelihood         int    `json:"likelihood"`
	Impact             int    `json:"impact"`
	ImpactDescription  string `json:"impact_description"`
	ImplementationDate string `json:"implementation_date"`
	ReassessmentDate   string `json:"reassessment_date"`

	// Step 3: Action Plan
	AssignmentTeamID      int                       `json:"assignment_team_id"`
	OwnerID               int                       `json:"owner_id"`
	ActionOwnerID         int                       `json:"action_owner_id"`
	ActionPlanDescription string                    `json:"action_plan_description"`
	ActionSteps           []CreateActionStepRequest `json:"action_steps"`
	TreatmentStrategy     string                    `json:"treatment_strategy"`
	Progress              string                    `json:"progress,omitempty"`
	GitIssueURL           string                    `json:"git_issue_url,omitempty"`
	EmailSubject          string                    `json:"email_subject"`
	Remarks               string                    `json:"remarks,omitempty"`
}

// CreateActionStepRequest represents one step in the action plan.
type CreateActionStepRequest struct {
	Description string `json:"description"`
}

// CreateRiskResponse is returned on successful POST /api/v1/risks.
type CreateRiskResponse struct {
	ID       int    `json:"id"`
	RiskCode string `json:"risk_code"`
}

// NextSequenceIDResponse is returned by GET /api/v1/risks/next-sequence-id.
type NextSequenceIDResponse struct {
	NextSequenceID int `json:"next_sequence_id"`
}

// ListRisksFilter holds query parameters for filtering the risk list.
type ListRisksFilter struct {
	Status   string
	TeamID   int
	Level    string
	Search   string
}

// UpdateRiskRequest is the payload for PUT /api/v1/risks/{id}.
type UpdateRiskRequest struct {
	RiskTitle         string  `json:"risk_title"`
	RiskDescription   string  `json:"risk_description"`
	ImpactDescription string  `json:"impact_description"`
	Progress          string  `json:"progress"`
	GitIssueURL       string  `json:"git_issue_url"`
	EmailSubject      string  `json:"email_subject"`
	Remarks           string  `json:"remarks"`
}

// RejectRiskRequest carries the mandatory rejection comment.
type RejectRiskRequest struct {
	RejectionComment string `json:"rejection_comment"`
}

// EscalateRiskRequest carries escalation details.
type EscalateRiskRequest struct {
	EscalatedTo int    `json:"escalated_to"`
	Reason      string `json:"reason"`
}

// AssessRiskRequest carries residual assessment values.
type AssessRiskRequest struct {
	NewTreatmentStrategy string `json:"new_treatment_strategy"`
	Decision             string `json:"decision"`
}

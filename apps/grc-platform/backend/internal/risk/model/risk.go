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
	ID                     int       `json:"id"`
	RiskYear               int       `json:"risk_year"`
	SourceRegisterID       int       `json:"source_register_id"`
	RiskQuarter            string    `json:"risk_quarter"`
	RiskCode               string    `json:"risk_code"`
	RiskTitle              string    `json:"risk_title"`
	RiskDescription        string    `json:"risk_description"`
	RiskIdentifiedDate     *string   `json:"risk_identified_date"`
	IdentifiedByType       *string   `json:"identified_by_type"`
	IdentifiedByUserID     *int      `json:"identified_by_user_id"`
	IdentifiedByName       *string   `json:"identified_by_name"`
	AssignerID             int       `json:"assigner_id"`
	OwnerID                int       `json:"owner_id"`
	ImpactDescription      *string   `json:"impact_description"`
	GrossScoreID           *int      `json:"gross_score_id"`
	TreatmentStrategy      *string   `json:"treatment_strategy"`
	ActionPlanID           *int      `json:"action_plan_id"`
	AssignmentTeamID       int       `json:"assignment_team_id"`
	Progress               *string   `json:"progress"`
	ImplementationDate     *string   `json:"implementation_date"`
	ReassessmentDate       *string   `json:"reassessment_date"`
	ComplianceApprovalBy   *int      `json:"compliance_approval_by"`
	ComplianceApprovalDate *string   `json:"compliance_approval_date"`
	GitIssueURL            *string   `json:"git_issue_url"`
	EmailSubject           *string   `json:"email_subject"`
	Remarks                *string   `json:"remarks"`
	WorkflowStatus         string    `json:"workflow_status"`
	RejectionComment       *string   `json:"rejection_comment"`
	CreatedAt              time.Time `json:"created_at"`
	CreatedBy              string    `json:"created_by"`
	UpdatedAt              time.Time `json:"updated_at"`
	UpdatedBy              string    `json:"updated_by"`
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

// ListRisksFilter holds query parameters for filtering and paginating the risk
// list. Every multi-value field is OR-matched within itself and AND-matched
// against the other fields (spreadsheet-style column filtering) — an empty
// slice/string means "no restriction on this field".
type ListRisksFilter struct {
	Statuses       []string // workflow_status values to include (empty = all)
	TeamIDs        []int    // source_register_id values to include (empty = all)
	Levels         []string // LOW / MEDIUM / HIGH values to include (empty = all)
	Search         string   // matched against risk_code and risk_title
	RiskTypes      []string // NEW / UPDATED values to include (empty = all)
	OwnerIDs       []int    // owner_id values to include (empty = all)
	SubmittedFrom  string   // created_at >= this date (YYYY-MM-DD); empty = unbounded
	SubmittedTo    string   // created_at <= this date (YYYY-MM-DD); empty = unbounded
	DueFrom        string   // implementation_date >= this date (YYYY-MM-DD); empty = unbounded
	DueTo          string   // implementation_date <= this date (YYYY-MM-DD); empty = unbounded
	DueOverdueOnly bool     // implementation_date < today, regardless of the range above
	Limit          int      // rows per page; handler enforces a sensible default and max
	Offset         int      // zero-based row offset
}

// RiskListPage is the paginated response for GET /api/v1/risks.
type RiskListPage struct {
	Items  []*RiskListItem `json:"items"`
	Total  int             `json:"total"`
	Offset int             `json:"offset"`
	Limit  int             `json:"limit"`
}

// RiskListItem is the lightweight DTO returned by GET /api/v1/risks.
// Joins resolve display names so the frontend table needs no secondary fetches.
type RiskListItem struct {
	ID                 int     `json:"id"`
	RiskCode           string  `json:"risk_code"`
	RiskTitle          string  `json:"risk_title"`
	SourceRegisterName string  `json:"source_register_name"`
	RiskLevel          string  `json:"risk_level"`
	RiskLevelColor     string  `json:"risk_level_color"`
	OwnerName          string  `json:"owner_name"`
	AssignerName       string  `json:"assigner_name"`
	WorkflowStatus     string  `json:"workflow_status"`
	RiskType           string  `json:"risk_type"`
	ImplementationDate *string `json:"implementation_date"`
	RejectionComment   *string `json:"rejection_comment"`
	RejectionStage     *string `json:"rejection_stage"`
	CreatedAt          string  `json:"created_at"`
}

// RiskDetail is the enriched DTO returned by GET /api/v1/risks/{id}.
// Includes all risk fields, resolved display names, and related entities.
type RiskDetail struct {
	// Core risk fields
	ID                     int     `json:"id"`
	RiskCode               string  `json:"risk_code"`
	RiskYear               int     `json:"risk_year"`
	RiskQuarter            string  `json:"risk_quarter"`
	RiskTitle              string  `json:"risk_title"`
	RiskDescription        string  `json:"risk_description"`
	RiskIdentifiedDate     *string `json:"risk_identified_date"`
	IdentifiedByType       *string `json:"identified_by_type"`
	IdentifiedByUserID     *int    `json:"identified_by_user_id"`
	IdentifiedByName       *string `json:"identified_by_name"`
	AssignerID             int     `json:"assigner_id"`
	OwnerID                int     `json:"owner_id"`
	ImpactDescription      *string `json:"impact_description"`
	TreatmentStrategy      *string `json:"treatment_strategy"`
	AssignmentTeamID       int     `json:"assignment_team_id"`
	Progress               *string `json:"progress"`
	ImplementationDate     *string `json:"implementation_date"`
	ReassessmentDate       *string `json:"reassessment_date"`
	GitIssueURL            *string `json:"git_issue_url"`
	EmailSubject           *string `json:"email_subject"`
	Remarks                *string `json:"remarks"`
	WorkflowStatus         string  `json:"workflow_status"`
	RiskType               string  `json:"risk_type"`
	RejectionComment       *string `json:"rejection_comment"`
	RejectionStage         *string `json:"rejection_stage"`
	OwnerFirstApprovedAt   *string `json:"owner_first_approved_at"`
	ComplianceApprovalDate *string `json:"compliance_approval_date"`
	CreatedAt              string  `json:"created_at"`
	UpdatedAt              string  `json:"updated_at"`

	// Resolved display names
	SourceRegisterName     string  `json:"source_register_name"`
	AssignmentTeamName     string  `json:"assignment_team_name"`
	OwnerName              string  `json:"owner_name"`
	AssignerName           string  `json:"assigner_name"`
	IdentifiedByUserName   *string `json:"identified_by_user_name"`
	ComplianceApproverName *string `json:"compliance_approver_name"`

	// Gross score (from risk_score join) — the original rating assigned at
	// creation, immutable once a risk owner has approved the risk. Used by
	// EditRiskDialog to pre-fill the edit form; do not repurpose for display
	// of the risk's current standing, see EffectiveScore.
	GrossScore *RiskScore `json:"gross_score"`
	// EffectiveScore is the risk's current residual score: the latest
	// reassessment's score when one exists, else the gross score — the same
	// "effective residual score" convention used by the dashboard/analytics
	// repositories. This is what tables and headers should display.
	EffectiveScore *RiskScore `json:"effective_score"`

	// Related entities
	ComplianceReferences []ComplianceReference `json:"compliance_references"`
	ActionPlan           *ActionPlanDetail     `json:"action_plan"`
	Assessments          []RiskAssessment      `json:"assessments"`
}

// ActionPlanDetail is ActionPlan with its steps embedded, used inside RiskDetail.
type ActionPlanDetail struct {
	ID            int              `json:"id"`
	ActionOwnerID *int             `json:"action_owner_id"`
	Description   *string          `json:"description"`
	Status        string           `json:"status"`
	PlanType      string           `json:"plan_type"`
	Steps         []ActionPlanStep `json:"steps"`
}

// UpdateRiskRequest is the payload for PUT /api/v1/risks/{id}.
// Three fields trigger PENDING_AMENDMENT when changed on an IN_REMEDIATION risk:
//   - ImplementationDate
//   - EmailSubject
//   - ActionSteps
//
// All other fields are free-edit and do not affect workflow status.
type UpdateRiskRequest struct {
	RiskTitle              string  `json:"risk_title"`
	RiskDescription        string  `json:"risk_description"`
	RiskIdentifiedDate     string  `json:"risk_identified_date,omitempty"`
	IdentifiedByType       string  `json:"identified_by_type,omitempty"`
	IdentifiedByUserID     *int    `json:"identified_by_user_id,omitempty"`
	IdentifiedByName       *string `json:"identified_by_name,omitempty"`
	AssignerID             *int    `json:"assigner_id,omitempty"`
	OwnerID                *int    `json:"owner_id,omitempty"`
	ImpactDescription      string  `json:"impact_description"`
	ComplianceReferenceIDs []int   `json:"compliance_reference_ids"`
	Progress               string  `json:"progress,omitempty"`
	GitIssueURL            string  `json:"git_issue_url,omitempty"`
	Remarks                string  `json:"remarks,omitempty"`
	TreatmentStrategy      string  `json:"treatment_strategy,omitempty"`
	AssignmentTeamID       *int    `json:"assignment_team_id,omitempty"`
	ActionPlanDescription  string  `json:"action_plan_description,omitempty"`
	ActionOwnerID          *int    `json:"action_owner_id,omitempty"`

	// RESTRICTED — trigger PENDING_AMENDMENT if changed on an IN_REMEDIATION risk
	ImplementationDate string                    `json:"implementation_date,omitempty"`
	EmailSubject       string                    `json:"email_subject"`
	ActionSteps        []UpdateActionStepRequest `json:"action_steps,omitempty"`

	// Full-edit only (editable before risk owner approval)
	ReassessmentDate string `json:"reassessment_date,omitempty"`
	GrossScoreID     *int   `json:"gross_score_id,omitempty"`
}

// UpdateActionStepRequest is one step inside UpdateRiskRequest.ActionSteps.
type UpdateActionStepRequest struct {
	ID          *int   `json:"id,omitempty"`
	Description string `json:"description"`
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

// TODO: escalation — for MEDIUM/HIGH risks past their implementation_date deadline,
// compliance can escalate to management via POST /api/v1/risks/{id}/escalate.
// Management responds by adding a MANAGEMENT action plan. See risk_escalation table.

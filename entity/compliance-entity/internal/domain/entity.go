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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

// Package domain defines all request/response types for the compliance entity service.
//
// Conventions:
//   - JSON field names use camelCase.
//   - All timestamp fields use the "On" suffix: createdOn, updatedOn.
//   - Optional response fields use pointer types so they serialise as JSON null.
//   - Enum filter fields in request structs use the "Key"/"Keys" suffix.
//   - Pagination.Limit is capped at 100 by the service layer.
package domain

import "time"

// =============================================================================
// Shared
// =============================================================================

// Pagination is embedded in every search request.
type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// =============================================================================
// User
// =============================================================================

// User represents a platform user from the shared `user` table.
// RiskTeamIDs comes from the user_risk_team join table (a user may belong to
// zero or more risk teams); AuditTeamID remains a single column on `user`.
type User struct {
	ID          int       `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	UserType    string    `json:"userType"` // INTERNAL | EXTERNAL
	AuditTeamID *int      `json:"auditTeamId"`
	RiskTeamIDs []int     `json:"riskTeamIds"`
	Status      string    `json:"status"`
	CreatedOn   time.Time `json:"createdOn"`
	UpdatedOn   time.Time `json:"updatedOn"`
}

// SearchUsersRequest is the payload for POST /users/search.
type SearchUsersRequest struct {
	SearchQuery string     `json:"searchQuery"`
	StatusKey   string     `json:"statusKey"` // ACTIVE | INACTIVE | REMOVED | "" (all)
	Pagination  Pagination `json:"pagination"`
}

// SearchUsersResponse is returned by POST /users/search.
type SearchUsersResponse struct {
	Users  []User `json:"users"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// =============================================================================
// Audit Team
// =============================================================================

// AuditTeam represents a team from the `audit_team` table.
type AuditTeam struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedOn time.Time `json:"createdOn"`
	UpdatedOn time.Time `json:"updatedOn"`
}

// SearchAuditTeamsRequest is the payload for POST /audit/teams/search.
type SearchAuditTeamsRequest struct {
	SearchQuery string     `json:"searchQuery"`
	StatusKey   string     `json:"statusKey"` // ACTIVE | INACTIVE | "" (all)
	Pagination  Pagination `json:"pagination"`
}

// SearchAuditTeamsResponse is returned by POST /audit/teams/search.
type SearchAuditTeamsResponse struct {
	Teams  []AuditTeam `json:"teams"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// =============================================================================
// Audit Framework
// =============================================================================

// AuditFramework represents a compliance framework from the `audit_framework` table.
type AuditFramework struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedOn time.Time `json:"createdOn"`
	UpdatedOn time.Time `json:"updatedOn"`
}

// SearchAuditFrameworksRequest is the payload for POST /audit/frameworks/search.
type SearchAuditFrameworksRequest struct {
	SearchQuery string     `json:"searchQuery"`
	StatusKey   string     `json:"statusKey"` // ACTIVE | INACTIVE | "" (all)
	Pagination  Pagination `json:"pagination"`
}

// SearchAuditFrameworksResponse is returned by POST /audit/frameworks/search.
type SearchAuditFrameworksResponse struct {
	Frameworks []AuditFramework `json:"frameworks"`
	Total      int              `json:"total"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

// =============================================================================
// Audit Framework Control (versioned control library)
// =============================================================================

// AuditFrameworkControl represents one immutable version of a control definition
// in the `audit_framework_control` table. Rows are never updated — a new version
// row is inserted instead. audit_control rows reference a specific version via
// framework_control_id.
type AuditFrameworkControl struct {
	ID                  int       `json:"id"`
	FrameworkID         int       `json:"frameworkId"`
	ControlNumber       string    `json:"controlNumber"`
	Description         string    `json:"description"`
	EvidenceRequirement *string   `json:"evidenceRequirement"`
	RequirementType     string    `json:"requirementType"`
	ControlType         string    `json:"controlType"`
	Scope               string    `json:"scope"`
	Version             int       `json:"version"`
	IsCurrent           bool      `json:"isCurrent"`
	CreatedOn           time.Time `json:"createdOn"`
	CreatedBy           *string   `json:"createdBy"`
}

// ListFrameworkControlsResponse is returned by GET /audit/frameworks/{id}/controls.
type ListFrameworkControlsResponse struct {
	Controls []AuditFrameworkControl `json:"controls"`
	Total    int                     `json:"total"`
}

// ListFrameworkControlVersionsResponse is returned by GET /audit/frameworks/{id}/controls/{controlNumber}/versions.
type ListFrameworkControlVersionsResponse struct {
	Versions []AuditFrameworkControl `json:"versions"`
}

// CreateFrameworkControlRequest is the payload for POST /audit/frameworks/{id}/controls.
type CreateFrameworkControlRequest struct {
	ControlNumber       string  `json:"controlNumber"`
	Description         string  `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     string  `json:"requirementType"`
	ControlType         string  `json:"controlType"`
	Scope               string  `json:"scope"`
	CreatedBy           string  `json:"createdBy"`
}

// UpdateFrameworkControlRequest is the payload for PUT /audit/frameworks/{id}/controls/{controlId}.
// Creates a new version row; the previous row is marked is_current=FALSE.
type UpdateFrameworkControlRequest struct {
	Description         *string `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     *string `json:"requirementType"`
	ControlType         *string `json:"controlType"`
	Scope               *string `json:"scope"`
	UpdatedBy           string  `json:"updatedBy"`
}

// =============================================================================
// Audit Product
// =============================================================================

// AuditProduct represents a product from the `audit_product` table.
type AuditProduct struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedOn time.Time `json:"createdOn"`
	UpdatedOn time.Time `json:"updatedOn"`
}

// SearchAuditProductsRequest is the payload for POST /audit/products/search.
type SearchAuditProductsRequest struct {
	SearchQuery string     `json:"searchQuery"`
	StatusKey   string     `json:"statusKey"` // ACTIVE | INACTIVE | "" (all)
	Pagination  Pagination `json:"pagination"`
}

// SearchAuditProductsResponse is returned by POST /audit/products/search.
type SearchAuditProductsResponse struct {
	Products []AuditProduct `json:"products"`
	Total    int            `json:"total"`
	Limit    int            `json:"limit"`
	Offset   int            `json:"offset"`
}

// =============================================================================
// Audit
// =============================================================================

// Audit represents an audit engagement from the `audit` table.
// Framework and product names are joined in to avoid extra round-trips.
type Audit struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	FrameworkID      int       `json:"frameworkId"`
	FrameworkName    string    `json:"frameworkName"`
	ProductID        int       `json:"productId"`
	ProductName      string    `json:"productName"`
	PeriodStart      string    `json:"periodStart"` // YYYY-MM-DD
	PeriodEnd        string    `json:"periodEnd"`   // YYYY-MM-DD
	Status           string    `json:"status"`
	ScopeDescription *string   `json:"scopeDescription"`
	ControlsTotal    int       `json:"controlsTotal"`
	ControlsApproved int       `json:"controlsApproved"`
	ControlsOverdue  int       `json:"controlsOverdue"`
	CreatedOn        time.Time `json:"createdOn"`
	UpdatedOn        time.Time `json:"updatedOn"`
}

// SearchAuditsRequest is the payload for POST /audits/search.
type SearchAuditsRequest struct {
	SearchQuery  string     `json:"searchQuery"`
	StatusKeys   []string   `json:"statusKeys"`   // ACTIVE | COMPLETED | ARCHIVED | REMOVED
	FrameworkIDs []int      `json:"frameworkIds"` // filter by one or more framework IDs
	ProductIDs   []int      `json:"productIds"`   // filter by one or more product IDs
	Pagination   Pagination `json:"pagination"`
}

// SearchAuditsResponse is returned by POST /audits/search.
type SearchAuditsResponse struct {
	Audits []Audit `json:"audits"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// =============================================================================
// Audit Control
// =============================================================================

// AuditControl represents a control from the `audit_control` table.
// Definition columns are resolved via COALESCE from audit_framework_control when linked.
// Owner, team, and auditor names are joined in.
type AuditControl struct {
	ID                  int       `json:"id"`
	AuditID             int       `json:"auditId"`
	FrameworkControlID  *int      `json:"frameworkControlId"` // non-nil when sourced from template
	TemplateVersion     *int      `json:"templateVersion"`    // version of the template row used
	ControlNumber       string    `json:"controlNumber"`
	Description         string    `json:"description"`
	EvidenceRequirement *string   `json:"evidenceRequirement"`
	RequirementType     string    `json:"requirementType"`
	ControlType         string    `json:"controlType"`
	Scope               string    `json:"scope"`
	OwnerID             *int      `json:"ownerId"`
	OwnerName           *string   `json:"ownerName"`
	TeamID              *int      `json:"teamId"`
	TeamName            *string   `json:"teamName"`
	AuditorID           *int      `json:"auditorId"`
	AuditorName         *string   `json:"auditorName"`
	DueDate             *string   `json:"dueDate"` // YYYY-MM-DD
	Status              string    `json:"status"`
	ControlSource       string    `json:"controlSource"` // MANUAL | COPIED | CSV
	IsOverdue           bool      `json:"isOverdue"`
	CreatedOn           time.Time `json:"createdOn"`
	UpdatedOn           time.Time `json:"updatedOn"`
	// Population-phase fields (OE controls only), from the initial audit_population record.
	PopulationDescription *string `json:"populationDescription"`
	PopulationComments    *string `json:"populationComments"`
	PopulationDueDate     *string `json:"populationDueDate"`
	PopulationOwnerName   *string `json:"populationOwnerName"`
	PopulationTeamName    *string `json:"populationTeamName"`
}

// SearchControlsRequest is the payload for POST /audits/{auditId}/controls/search.
type SearchControlsRequest struct {
	SearchQuery      string     `json:"searchQuery"`
	StatusKeys       []string   `json:"statusKeys"`       // control status values
	RequirementTypes []string   `json:"requirementTypes"` // DESIGN | OE
	TeamIDs          []int      `json:"teamIds"`
	AuditorIDs       []int      `json:"auditorIds"` // filter by assigned auditor user IDs
	OwnerIDs         []int      `json:"ownerIds"`   // filter by assigned owner user IDs
	Pagination       Pagination `json:"pagination"`
}

// SearchControlsResponse is returned by POST /audits/{auditId}/controls/search.
type SearchControlsResponse struct {
	Controls []AuditControl `json:"controls"`
	Total    int            `json:"total"`
	Limit    int            `json:"limit"`
	Offset   int            `json:"offset"`
}

// AssignedControlForEvidence is a control a user's team must submit evidence for.
// It is enriched with audit/product/framework so the Evidence Portal can render a
// control without extra round-trips. The phase-aware base folder path is computed
// by the GRC Backend (never trusted from a client), so it is not carried here.
type AssignedControlForEvidence struct {
	AuditID             int     `json:"auditId"`
	AuditName           string  `json:"auditName"`
	Product             string  `json:"product"`
	Framework           string  `json:"framework"`
	PeriodStart         string  `json:"periodStart"` // YYYY-MM-DD
	PeriodEnd           string  `json:"periodEnd"`   // YYYY-MM-DD
	ControlID           int     `json:"controlId"`
	ControlNumber       string  `json:"controlNumber"`
	Description         string  `json:"description"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	RequirementType     string  `json:"requirementType"` // DESIGN | OE
	Status              string  `json:"status"`
	DueDate             *string `json:"dueDate"` // YYYY-MM-DD
}

// ListAssignedControlsResponse is returned by GET /controls/assigned-for-evidence.
type ListAssignedControlsResponse struct {
	Controls []AssignedControlForEvidence `json:"controls"`
}

// EvidenceAssignmentResponse is returned by
// GET /audit-controls/{controlId}/evidence-assignment?email=. A 200 with the
// derived audit id means the user is assigned to this control right now (for
// either the population or evidence phase); a 404 means not assigned.
type EvidenceAssignmentResponse struct {
	AuditID int `json:"auditId"`
}

// ActivePopulationResponse is returned by
// GET /audit-controls/{controlId}/active-population. A 200 carries the id of the
// population round the team must act on (status PENDING or COMPLIANCE_REJECTED);
// a 404 means no active population (e.g. a DESIGN control).
type ActivePopulationResponse struct {
	PopulationID int `json:"populationId"`
}

// BulkCreateControlsRequest is the payload for POST /audits/{auditId}/controls/bulk.
type BulkCreateControlsRequest struct {
	Controls []CreateControlRequest `json:"controls"`
}

// BulkCreateControlsResponse is returned by POST /audits/{auditId}/controls/bulk.
type BulkCreateControlsResponse struct {
	Controls []AuditControl `json:"controls"`
	Created  int            `json:"created"`
}

// =============================================================================
// Risk Team
// =============================================================================

// RiskTeam represents a team from the `risk_team` table.
// team_type determines which UI pickers show this team.
type RiskTeam struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Code        *string   `json:"code"`
	Description *string   `json:"description"`
	TeamType    string    `json:"teamType"` // SOURCE_REGISTER | ASSIGNMENT | BOTH
	Status      string    `json:"status"`
	CreatedOn   time.Time `json:"createdOn"`
	UpdatedOn   time.Time `json:"updatedOn"`
}

// SearchRiskTeamsRequest is the payload for POST /risk/teams/search.
type SearchRiskTeamsRequest struct {
	SearchQuery  string     `json:"searchQuery"`
	TeamTypeKeys []string   `json:"teamTypeKeys"` // SOURCE_REGISTER | ASSIGNMENT | BOTH
	StatusKey    string     `json:"statusKey"`    // ACTIVE | INACTIVE | REMOVED | "" (all)
	Pagination   Pagination `json:"pagination"`
}

// SearchRiskTeamsResponse is returned by POST /risk/teams/search.
type SearchRiskTeamsResponse struct {
	Teams  []RiskTeam `json:"teams"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// =============================================================================
// Risk Score
// =============================================================================

// RiskScore represents one of the 9 likelihood×impact combinations.
type RiskScore struct {
	ID         int    `json:"id"`
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskRating int    `json:"riskRating"`
	RiskLevel  string `json:"riskLevel"` // LOW | MEDIUM | HIGH
	ColorCode  string `json:"colorCode"` // hex colour
}

// ListRiskScoresResponse is returned by GET /risk/scores.
type ListRiskScoresResponse struct {
	Scores []RiskScore `json:"scores"`
}

// ListRiskCategoriesResponse is returned by GET /risk/categories.
type ListRiskCategoriesResponse struct {
	Categories []RiskCategory `json:"categories"`
}

// =============================================================================
// Risk Compliance Reference
// =============================================================================

// RiskComplianceReference represents a security/compliance framework
// that risks can be tagged against (e.g. ISO 27001, SOC 2, PCI DSS).
type RiskComplianceReference struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedOn   time.Time `json:"createdOn"`
	UpdatedOn   time.Time `json:"updatedOn"`
}

// RiskCategory represents a risk_category lookup row (e.g. "PII / Sensitive
// Data Exposure"). Linked to risks via risk_category_reference, which is
// genuinely many-to-many at the schema level even though only one row is
// ever written per risk today.
type RiskCategory struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// SearchRiskReferencesRequest is the payload for POST /risk/compliance-references/search.
type SearchRiskReferencesRequest struct {
	SearchQuery string     `json:"searchQuery"`
	Pagination  Pagination `json:"pagination"`
}

// SearchRiskReferencesResponse is returned by POST /risk/compliance-references/search.
type SearchRiskReferencesResponse struct {
	References []RiskComplianceReference `json:"references"`
	Total      int                       `json:"total"`
	Limit      int                       `json:"limit"`
	Offset     int                       `json:"offset"`
}

// =============================================================================
// Risk
// =============================================================================

// Risk represents a risk record from the `risk` table.
// Joined fields (source register name, assignment team name, assigner/owner names)
// are included to avoid extra round-trips.
type Risk struct {
	ID                 int       `json:"id"`
	RiskCode           string    `json:"riskCode"`
	RiskYear           int       `json:"riskYear"`
	RiskQuarter        string    `json:"riskQuarter"`
	RiskTitle          string    `json:"riskTitle"`
	RiskDescription    *string   `json:"riskDescription"`
	SourceRegisterID   int       `json:"sourceRegisterId"`
	SourceRegisterName string    `json:"sourceRegisterName"`
	AssignmentTeamID   int       `json:"assignmentTeamId"`
	AssignmentTeamName string    `json:"assignmentTeamName"`
	AssignerID         int       `json:"assignerId"`
	AssignerName       string    `json:"assignerName"`
	OwnerID            int       `json:"ownerId"`
	OwnerName          string    `json:"ownerName"`
	// ManagementApproverID names the user who approves this risk during
	// PENDING_MANAGEMENT_APPROVAL and is the target an ESCALATED risk
	// conceptually escalates to. Required on every risk regardless of level
	// or treatment strategy.
	ManagementApproverID   int       `json:"managementApproverId"`
	ManagementApproverName string    `json:"managementApproverName"`
	WorkflowStatus         string    `json:"workflowStatus"`
	TreatmentStrategy  *string   `json:"treatmentStrategy"`
	GrossScoreID       *int      `json:"grossScoreId"`
	GrossRiskLevel     *string   `json:"grossRiskLevel"`
	ImplementationDate *string   `json:"implementationDate"` // YYYY-MM-DD
	ReassessmentDate   *string   `json:"reassessmentDate"`   // YYYY-MM-DD
	CreatedOn          time.Time `json:"createdOn"`
	UpdatedOn          time.Time `json:"updatedOn"`

	// Remaining risk columns. These were absent while nothing consumed this
	// type; the GRC backend's risk detail and list views need all of them, and
	// omitting one renders as a blank field rather than an error.
	RiskIdentifiedDate     *string `json:"riskIdentifiedDate"` // YYYY-MM-DD
	IdentifiedByType       *string `json:"identifiedByType"`   // EMPLOYEE | EXTERNAL_PERSON | TOOL
	IdentifiedByName       *string `json:"identifiedByName"`
	ImpactDescription      *string `json:"impactDescription"`
	ActionPlanID           *int    `json:"actionPlanId"`
	Progress               *string `json:"progress"`
	ComplianceApprovalBy   *int    `json:"complianceApprovalBy"`
	ComplianceApprovalDate *string `json:"complianceApprovalDate"` // YYYY-MM-DD
	GitIssueURL            *string `json:"gitIssueUrl"`
	EmailSubject           *string `json:"emailSubject"`
	Remarks                *string `json:"remarks"`
	RiskType               string  `json:"riskType"` // NEW | UPDATED
	RejectionComment       *string `json:"rejectionComment"`
	RejectionStage         *string `json:"rejectionStage"`
	OwnerFirstApprovedAt   *string `json:"ownerFirstApprovedAt"`
	CreatedBy              string  `json:"createdBy"`
	UpdatedBy              string  `json:"updatedBy"`

	// Effective residual standing: the most recent assessment's score, or the
	// gross score when the risk has not been reassessed. This is what a risk
	// row should display — GrossRiskLevel is the original rating and goes stale
	// the moment a reassessment lands.
	EffectiveRiskLevel *string `json:"effectiveRiskLevel"`
	EffectiveColorCode *string `json:"effectiveColorCode"`
}

// SearchRisksRequest is the payload for POST /risks/search.
type SearchRisksRequest struct {
	SearchQuery        string   `json:"searchQuery"` // matched against risk_code and risk_title
	WorkflowStatusKeys []string `json:"workflowStatusKeys"`
	SourceRegisterIDs  []int    `json:"sourceRegisterIds"`
	AssignmentTeamIDs  []int    `json:"assignmentTeamIds"`
	RiskYears          []int    `json:"riskYears"`
	RiskQuarterKeys    []string `json:"riskQuarterKeys"` // Q1 | Q2 | Q3 | Q4

	// RiskLevelKeys filters on the *effective* residual level — the most recent
	// assessment's level, or the gross level when a risk has not been
	// reassessed. Filtering on the gross level would contradict what the same
	// row displays.
	RiskLevelKeys []string `json:"riskLevelKeys"` // LOW | MEDIUM | HIGH
	RiskTypeKeys  []string `json:"riskTypeKeys"`  // NEW | UPDATED
	OwnerIDs      []int    `json:"ownerIds"`
	// ActionOwnerID restricts to risks with at least one risk_action_plan row
	// (STANDARD or MANAGEMENT) whose action_owner_id matches — how the Action
	// Owner's risk list is scoped to only what they're assigned to.
	ActionOwnerID *int `json:"actionOwnerId"`
	// ScopeTeamIDs restricts to risks whose source register or assignment team
	// is one of these — how a Risk Assigner/Risk Owner-only caller's list is
	// scoped to their own risk teams. Empty means unrestricted; the GRC backend
	// decides whether a caller needs this at all before populating it.
	ScopeTeamIDs []int `json:"scopeTeamIds"`

	// Submitted* bound created_at, Due* bound implementation_date. Dates are
	// YYYY-MM-DD and inclusive at both ends.
	SubmittedFrom string `json:"submittedFrom"`
	SubmittedTo   string `json:"submittedTo"`
	DueFrom       string `json:"dueFrom"`
	DueTo         string `json:"dueTo"`
	// DueOverdueOnly restricts to risks already past their implementation date,
	// independent of any Due range above.
	DueOverdueOnly bool `json:"dueOverdueOnly"`

	Pagination Pagination `json:"pagination"`
}

// SearchRisksResponse is returned by POST /risks/search.
type SearchRisksResponse struct {
	Risks  []Risk `json:"risks"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// =============================================================================
// Write request types — User
// =============================================================================

// CreateUserRequest is the payload for POST /users.
type CreateUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	UserType    string `json:"userType"` // INTERNAL | EXTERNAL; defaults to INTERNAL
	AuditTeamID *int   `json:"auditTeamId"`
	RiskTeamIDs []int  `json:"riskTeamIds"`
	Status      string `json:"status"`
	CreatedBy   string `json:"createdBy"`
}

// UpdateUserRequest is the payload for PATCH /users/{id}.
// RiskTeamIDs is a pointer to a slice, not a plain slice, so a caller can omit
// it to leave team membership untouched — a nil pointer means "not provided,"
// while a non-nil (possibly empty) slice means "replace membership with exactly
// this set." Every other field follows the same nil-means-omitted convention.
type UpdateUserRequest struct {
	DisplayName *string `json:"displayName"`
	UserType    *string `json:"userType"` // INTERNAL | EXTERNAL
	AuditTeamID *int    `json:"auditTeamId"`
	RiskTeamIDs *[]int  `json:"riskTeamIds"`
	Status      *string `json:"status"`
	UpdatedBy   string  `json:"updatedBy"`
}

// =============================================================================
// Write request types — Audit Team
// =============================================================================

// CreateAuditTeamRequest is the payload for POST /audit/teams.
type CreateAuditTeamRequest struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedBy string `json:"createdBy"`
}

// UpdateAuditTeamRequest is the payload for PATCH /audit/teams/{id}.
type UpdateAuditTeamRequest struct {
	Name      *string `json:"name"`
	Status    *string `json:"status"`
	UpdatedBy string  `json:"updatedBy"`
}

// =============================================================================
// Write request types — Audit Framework
// =============================================================================

// CreateAuditFrameworkRequest is the payload for POST /audit/frameworks.
type CreateAuditFrameworkRequest struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedBy string `json:"createdBy"`
}

// UpdateAuditFrameworkRequest is the payload for PATCH /audit/frameworks/{id}.
type UpdateAuditFrameworkRequest struct {
	Name      *string `json:"name"`
	Status    *string `json:"status"`
	UpdatedBy string  `json:"updatedBy"`
}

// =============================================================================
// Write request types — Audit Product
// =============================================================================

// CreateAuditProductRequest is the payload for POST /audit/products.
type CreateAuditProductRequest struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedBy string `json:"createdBy"`
}

// UpdateAuditProductRequest is the payload for PATCH /audit/products/{id}.
type UpdateAuditProductRequest struct {
	Name      *string `json:"name"`
	Status    *string `json:"status"`
	UpdatedBy string  `json:"updatedBy"`
}

// =============================================================================
// Write request types — Audit
// =============================================================================

// CreateAuditRequest is the payload for POST /audits.
type CreateAuditRequest struct {
	Name             string  `json:"name"`
	FrameworkID      int     `json:"frameworkId"`
	ProductID        int     `json:"productId"`
	PeriodStart      string  `json:"periodStart"` // YYYY-MM-DD
	PeriodEnd        string  `json:"periodEnd"`   // YYYY-MM-DD
	ScopeDescription *string `json:"scopeDescription"`
	CreatedBy        string  `json:"createdBy"`
}

// UpdateAuditRequest is the payload for PATCH /audits/{id}.
type UpdateAuditRequest struct {
	Name             *string `json:"name"`
	Status           *string `json:"status"`
	PeriodStart      *string `json:"periodStart"`
	PeriodEnd        *string `json:"periodEnd"`
	ScopeDescription *string `json:"scopeDescription"`
	UpdatedBy        string  `json:"updatedBy"`
}

// =============================================================================
// Write request types — Audit Control
// =============================================================================

// InlinePopulationRequest carries optional population data alongside an OE
// control creation request. Mirrors PopulationDetails on the backend side.
type InlinePopulationRequest struct {
	Description     string  `json:"description"`
	ReferenceNumber *int    `json:"referenceNumber"`
	DueDate         *string `json:"dueDate"`
	Comments        *string `json:"comments"`
	OwnerID         *int    `json:"ownerId"`
	TeamID          *int    `json:"teamId"`
}

// CreateControlRequest is the payload for POST /audits/{auditId}/controls.
type CreateControlRequest struct {
	// When FrameworkControlID is set the definition columns below may be omitted;
	// they will be resolved from the template via COALESCE on read.
	FrameworkControlID  *int                     `json:"frameworkControlId"`
	ControlSource       string                   `json:"controlSource"` // MANUAL | COPIED | CSV; defaults to MANUAL
	ControlNumber       string                   `json:"controlNumber"`
	Description         string                   `json:"description"`
	EvidenceRequirement *string                  `json:"evidenceRequirement"`
	RequirementType     string                   `json:"requirementType"` // DESIGN | OE
	ControlType         string                   `json:"controlType"`     // CONFIG | NON_CONFIG
	Scope               string                   `json:"scope"`           // COMMON | PRODUCT_SPECIFIC
	OwnerID             *int                     `json:"ownerId"`
	TeamID              *int                     `json:"teamId"`
	AuditorID           *int                     `json:"auditorId"`
	DueDate             *string                  `json:"dueDate"`    // YYYY-MM-DD
	Population          *InlinePopulationRequest `json:"population"` // OE controls only
	CreatedBy           string                   `json:"createdBy"`
}

// UpdateControlRequest is the payload for PATCH /audits/{auditId}/controls/{controlId}.
type UpdateControlRequest struct {
	Description         *string `json:"description"`
	ControlType         *string `json:"controlType"`
	Scope               *string `json:"scope"`
	EvidenceRequirement *string `json:"evidenceRequirement"`
	OwnerID             *int    `json:"ownerId"`
	TeamID              *int    `json:"teamId"`
	AuditorID           *int    `json:"auditorId"`
	DueDate             *string `json:"dueDate"`
	Status              *string `json:"status"`
	Comments            *string `json:"comments"`
	SampleReference     *string `json:"sampleReference"`
	UpdatedBy           string  `json:"updatedBy"`
	ExpectedStatus      string  `json:"-"` // set server-side for atomic transition; never decoded from JSON
}

// =============================================================================
// Evidence (audit_evidence + audit_evidence_file)
// =============================================================================

// AuditEvidence is a single evidence submission row.
type AuditEvidence struct {
	ID                   int       `json:"id"`
	ControlID            int       `json:"controlId"`
	SubmittedBy          *int      `json:"submittedBy"`
	Status               string    `json:"status"`
	FolderPath           *string   `json:"folderPath"`
	ReusedFromEvidenceID *int      `json:"reusedFromEvidenceId"`
	CreatedBy            *string   `json:"createdBy"`
	CreatedOn            time.Time `json:"createdOn"`
	UpdatedOn            time.Time `json:"updatedOn"`
}

// CreateEvidenceRequest is the payload for POST /audits/{auditId}/controls/{controlId}/evidence.
type CreateEvidenceRequest struct {
	SubmittedBy          *int    `json:"submittedBy"`
	FolderPath           *string `json:"folderPath"`
	ReusedFromEvidenceID *int    `json:"reusedFromEvidenceId"`
	CreatedBy            string  `json:"createdBy"`
}

// UpdateEvidenceRequest is the payload for PATCH /evidence/{evidenceId}.
type UpdateEvidenceRequest struct {
	Status         string `json:"status"` // SUBMITTED | COMPLIANCE_APPROVED | COMPLIANCE_REJECTED | APPROVED | AUDITOR_REJECTED
	UpdatedBy      string `json:"updatedBy"`
	ExpectedStatus string `json:"-"` // set server-side for atomic transition; never decoded from JSON
}

// AuditEvidenceFile is one uploaded file attached to an evidence submission or population.
type AuditEvidenceFile struct {
	ID           int       `json:"id"`
	EvidenceID   *int      `json:"evidenceId"`
	PopulationID *int      `json:"populationId"`
	FileKind     *string   `json:"fileKind"` // POPULATION | SAMPLE (only when populationId is set)
	UploadedBy   *int      `json:"uploadedBy"`
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	FileType     *string   `json:"fileType"`
	FileSize     *int64    `json:"fileSize"`
	CreatedOn    time.Time `json:"createdOn"`
}

// CreateEvidenceFileRequest is the payload for POST /evidence/{evidenceId}/files.
type CreateEvidenceFileRequest struct {
	FileName   string  `json:"fileName"`
	FilePath   string  `json:"filePath"` // Azure Blob URL
	FileType   *string `json:"fileType"`
	FileSize   *int64  `json:"fileSize"`
	UploadedBy *int    `json:"uploadedBy"`
	CreatedBy  string  `json:"createdBy"`
}

// ListEvidenceFilesResponse is returned by GET /evidence/{evidenceId}/files.
type ListEvidenceFilesResponse struct {
	Files []AuditEvidenceFile `json:"files"`
}

// =============================================================================
// Blob file responses (POST /files, GET /files/list) — used by FileHandler
// =============================================================================

// UploadFileResponse is returned by POST /files.
type UploadFileResponse struct {
	BlobName string `json:"blobName"`
	Size     int    `json:"size"`
}

// BlobFileItem describes one blob entry in a folder listing.
type BlobFileItem struct {
	Name        string `json:"name"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

// ListFilesResponse is returned by GET /files/list.
type ListFilesResponse struct {
	Files []BlobFileItem `json:"files"`
}

// ListEvidenceResponse is returned by GET /audits/{auditId}/controls/{controlId}/evidence.
type ListEvidenceResponse struct {
	Evidence []AuditEvidence `json:"evidence"`
}

// =============================================================================
// Population (audit_population + population files in audit_evidence_file)
// =============================================================================

// AuditPopulation is the population record for an OE-type control.
type AuditPopulation struct {
	ID              int       `json:"id"`
	ControlID       int       `json:"controlId"`
	OwnerID         *int      `json:"ownerId"`
	TeamID          *int      `json:"teamId"`
	ReferenceNumber *int      `json:"referenceNumber"`
	Description     *string   `json:"description"`
	Status          string    `json:"status"`
	DueDate         *string   `json:"dueDate"`
	Comments        *string   `json:"comments"`
	CreatedOn       time.Time `json:"createdOn"`
	UpdatedOn       time.Time `json:"updatedOn"`
}

// CreatePopulationRequest is the payload for POST /audits/{auditId}/controls/{controlId}/populations.
type CreatePopulationRequest struct {
	OwnerID         *int    `json:"ownerId"`
	TeamID          *int    `json:"teamId"`
	ReferenceNumber *int    `json:"referenceNumber"`
	Description     *string `json:"description"`
	DueDate         *string `json:"dueDate"`
	CreatedBy       string  `json:"createdBy"`
}

// UpdatePopulationRequest is the payload for PATCH /populations/{populationId}.
type UpdatePopulationRequest struct {
	OwnerID         *int    `json:"ownerId"`
	TeamID          *int    `json:"teamId"`
	Status          *string `json:"status"`
	Comments        *string `json:"comments"`
	ReferenceNumber *int    `json:"referenceNumber"`
	Description     *string `json:"description"`
	DueDate         *string `json:"dueDate"`
	UpdatedBy       string  `json:"updatedBy"`
	ExpectedStatus  string  `json:"-"` // set server-side for atomic transition; never decoded from JSON
}

// CreatePopulationFileRequest attaches a file to a population record.
// FileKind must be POPULATION (team upload) or SAMPLE (auditor sample).
type CreatePopulationFileRequest struct {
	FileKind   string  `json:"fileKind"` // POPULATION | SAMPLE
	FileName   string  `json:"fileName"`
	FilePath   string  `json:"filePath"` // Azure Blob URL
	FileType   *string `json:"fileType"`
	FileSize   *int64  `json:"fileSize"`
	UploadedBy *int    `json:"uploadedBy"`
	CreatedBy  string  `json:"createdBy"`
}

// =============================================================================
// Write request types — Risk Team
// =============================================================================

// CreateRiskTeamRequest is the payload for POST /risk/teams.
type CreateRiskTeamRequest struct {
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
	TeamType    string  `json:"teamType"` // SOURCE_REGISTER | ASSIGNMENT | BOTH
	Status      string  `json:"status"`
	CreatedBy   string  `json:"createdBy"`
}

// UpdateRiskTeamRequest is the payload for PATCH /risk/teams/{id}.
type UpdateRiskTeamRequest struct {
	Name        *string `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
	TeamType    *string `json:"teamType"`
	Status      *string `json:"status"`
	UpdatedBy   string  `json:"updatedBy"`
}

// =============================================================================
// Write request types — Risk Compliance Reference
// =============================================================================

// CreateRiskReferenceRequest is the payload for POST /risk/compliance-references.
type CreateRiskReferenceRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	CreatedBy   string  `json:"createdBy"`
}

// UpdateRiskReferenceRequest is the payload for PATCH /risk/compliance-references/{id}.
type UpdateRiskReferenceRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	UpdatedBy   string  `json:"updatedBy"`
}

// =============================================================================
// Write request types — Risk
// =============================================================================

// CreateRiskRequest is the payload for POST /risks.
type CreateRiskRequest struct {
	RiskTitle        string  `json:"riskTitle"`
	RiskDescription  *string `json:"riskDescription"`
	SourceRegisterID int     `json:"sourceRegisterId"`
	AssignmentTeamID int     `json:"assignmentTeamId"`
	AssignerID       int     `json:"assignerId"`
	OwnerID          int     `json:"ownerId"`
	// ManagementApproverID is required on every risk regardless of level or
	// treatment strategy — see Risk.ManagementApproverID.
	ManagementApproverID int    `json:"managementApproverId"`
	RiskYear             int    `json:"riskYear"`
	RiskQuarter          string `json:"riskQuarter"` // Q1 | Q2 | Q3 | Q4
	// Likelihood and impact identify the gross score cell; the score_id is
	// resolved server-side from risk_score, as it is for assessments. Callers
	// describe the rating they gave, not the surrogate key behind it.
	Likelihood         int     `json:"likelihood"`
	Impact             int     `json:"impact"`
	TreatmentStrategy  *string `json:"treatmentStrategy"`
	ImplementationDate *string `json:"implementationDate"` // YYYY-MM-DD
	ReassessmentDate   *string `json:"reassessmentDate"`   // YYYY-MM-DD
	ImpactDescription  *string `json:"impactDescription"`
	RiskIdentifiedDate *string `json:"riskIdentifiedDate"`
	IdentifiedByType   *string `json:"identifiedByType"` // EMPLOYEE | EXTERNAL_PERSON | TOOL
	// IdentifiedByUserID is deliberately absent: the GRC platform dropped the
	// risk.identified_by_user_id column, and risks now record only
	// identified_by_name.
	IdentifiedByName *string `json:"identifiedByName"`
	GitIssueURL      *string `json:"gitIssueUrl"`
	EmailSubject     *string `json:"emailSubject"`
	Remarks          *string `json:"remarks"`
	Progress         *string `json:"progress"`

	// Creating a risk also creates its action plan, that plan's steps and its
	// compliance-reference links. They belong to this request rather than to
	// follow-up calls, so the whole thing commits or none of it does: a risk
	// that reaches the register without its action plan is not a valid state,
	// and over HTTP a second call can always fail.
	ActionOwnerID          *int              `json:"actionOwnerId"`
	ActionPlanDescription  *string           `json:"actionPlanDescription"`
	ActionSteps            []ActionStepInput `json:"actionSteps"`
	ComplianceReferenceIDs []int             `json:"complianceReferenceIds"`
	// RiskCategoryIDs writes risk_category_reference rows. The schema is
	// genuinely many-to-many (no DB constraint limits this to one row); the
	// GRC frontend only ever sends one today via a single-select dropdown.
	RiskCategoryIDs []int `json:"riskCategoryIds"`

	CreatedBy string `json:"createdBy"`
}

// ActionStepInput is one step of the action plan created alongside a risk.
// Step numbers are assigned from the slice order, starting at 1.
type ActionStepInput struct {
	Description string `json:"description"`
}

// UpdateRiskRequest is the payload for PATCH /risks/{id}.
type UpdateRiskRequest struct {
	RiskTitle              *string `json:"riskTitle"`
	RiskDescription        *string `json:"riskDescription"`
	WorkflowStatus         *string `json:"workflowStatus"`
	TreatmentStrategy      *string `json:"treatmentStrategy"`
	GrossScoreID           *int    `json:"grossScoreId"`
	ImplementationDate     *string `json:"implementationDate"`
	ReassessmentDate       *string `json:"reassessmentDate"`
	Progress               *string `json:"progress"`
	RejectionComment       *string `json:"rejectionComment"`
	RejectionStage         *string `json:"rejectionStage"`
	ComplianceApprovalBy   *int    `json:"complianceApprovalBy"`
	ComplianceApprovalDate *string `json:"complianceApprovalDate"`
	AssignmentTeamID       *int    `json:"assignmentTeamId"`
	OwnerID                *int    `json:"ownerId"`
	ManagementApproverID   *int    `json:"managementApproverId"`
	ActionPlanID           *int    `json:"actionPlanId"`
	GitIssueURL            *string `json:"gitIssueUrl"`
	Remarks                *string `json:"remarks"`
	// EmailSubject is settable on update: the backend treats a change to it as
	// one of the three edits that move an IN_REMEDIATION risk to
	// PENDING_AMENDMENT, so it must be updatable, not create-only.
	EmailSubject *string `json:"emailSubject"`
	// RiskType and OwnerFirstApprovedAt back the backend's SetRiskType and
	// SetOwnerFirstApprovedAt, which are single-column updates rather than
	// workflow transitions.
	RiskType             *string `json:"riskType"` // NEW | UPDATED
	OwnerFirstApprovedAt *string `json:"ownerFirstApprovedAt"`

	// Fields the risk edit form can change that were previously absent here.
	ImpactDescription  *string `json:"impactDescription"`
	RiskIdentifiedDate *string `json:"riskIdentifiedDate"` // YYYY-MM-DD
	IdentifiedByType   *string `json:"identifiedByType"`   // EMPLOYEE | EXTERNAL_PERSON | TOOL
	IdentifiedByName   *string `json:"identifiedByName"`
	AssignerID         *int    `json:"assignerId"`

	// ClearRejection sets rejection_comment and rejection_stage back to NULL.
	// A *string cannot express this: nil means "leave alone", so there is
	// otherwise no way to clear a nullable column — sending "" writes an empty
	// string, which is not the same thing and shows up as a blank rejection
	// banner rather than none.
	ClearRejection bool `json:"clearRejection"`

	UpdatedBy string `json:"updatedBy"`

	// Related rows the caller wants rewritten in the same transaction. Each is
	// nil when the caller is not touching that relation — an empty slice is a
	// meaningful instruction ("remove them all") and is not the same as nil.
	//
	// The caller decides *what* to write; this service decides whether the
	// write is legal and makes it atomic. That split matters: which edits
	// require re-approval, and what belongs in the change log, are workflow
	// rules owned by the GRC backend, not persistence rules owned here.
	ComplianceReferenceIDs []int              `json:"complianceReferenceIds"`
	RiskCategoryIDs        []int              `json:"riskCategoryIds"`
	ActionPlan             *ActionPlanUpdate  `json:"actionPlan"`
	ActionSteps            []ActionStepUpdate `json:"actionSteps"`
	ChangeLog              []ChangeLogEntry   `json:"changeLog"`

	// ExpectedStatus makes the update a compare-and-set. When the caller
	// supplies it, the UPDATE is guarded by that status and a mismatch is a
	// 409 — so a caller that read the risk, decided something, and is now
	// writing cannot be overtaken in between. Left empty, this service reads
	// the current status itself when a workflow transition is requested.
	ExpectedStatus string `json:"expectedStatus"`
}

// ActionPlanUpdate patches the risk's STANDARD action plan. Nil fields are left
// as they are.
type ActionPlanUpdate struct {
	Description   *string `json:"description"`
	ActionOwnerID *int    `json:"actionOwnerId"`
}

// ActionStepUpdate is one step in the desired final state of the action plan.
// A step carrying an ID that still exists on the plan is updated in place,
// which is what preserves its status and completed_date; anything else is
// inserted as new, and steps absent from the list are deleted. Step numbers are
// reassigned from list order.
type ActionStepUpdate struct {
	ID          *int   `json:"id"`
	Description string `json:"description"`
}

// ChangeLogEntry is one row for risk_change_log, composed by the caller because
// deciding what counts as a noteworthy change is a workflow question.
type ChangeLogEntry struct {
	Action       string  `json:"action"` // CREATE | UPDATE
	FieldChanged *string `json:"fieldChanged"`
	OldValue     *string `json:"oldValue"`
	NewValue     *string `json:"newValue"`
}

// =============================================================================
// Risk Action Plan
// =============================================================================

// RiskActionPlan is an action plan attached to a risk.
type RiskActionPlan struct {
	ID            int       `json:"id"`
	RiskID        int       `json:"riskId"`
	ActionOwnerID *int      `json:"actionOwnerId"`
	Description   *string   `json:"description"`
	Status        string    `json:"status"` // PENDING | IN_PROGRESS | COMPLETED
	CompletedDate *string   `json:"completedDate"`
	PlanType      string    `json:"planType"` // STANDARD | MANAGEMENT
	CreatedBy     *string   `json:"createdBy"`
	CreatedOn     time.Time `json:"createdOn"`
	UpdatedOn     time.Time `json:"updatedOn"`
}

// CreateRiskActionPlanRequest is the payload for POST /risks/{riskId}/action-plans.
type CreateRiskActionPlanRequest struct {
	Description   *string `json:"description"`
	ActionOwnerID *int    `json:"actionOwnerId"`
	PlanType      string  `json:"planType"` // STANDARD | MANAGEMENT
	CreatedBy     string  `json:"createdBy"`
	// Steps are the plan's action-step descriptions, created atomically with
	// the plan in the same transaction (see CreateRiskActionPlan) so a
	// partially-populated plan can never be persisted. Ordered; step_no is
	// assigned by position.
	Steps []string `json:"steps"`
}

// UpdateRiskActionPlanRequest is the payload for PATCH /action-plans/{planId}.
type UpdateRiskActionPlanRequest struct {
	Description   *string `json:"description"`
	ActionOwnerID *int    `json:"actionOwnerId"`
	Status        *string `json:"status"`
	CompletedDate *string `json:"completedDate"`
	UpdatedBy     string  `json:"updatedBy"`
}

// ListRiskActionPlansResponse is returned by GET /risks/{riskId}/action-plans.
type ListRiskActionPlansResponse struct {
	Plans []RiskActionPlan `json:"plans"`
}

// CompleteRiskActionPlanRequest is the payload for POST /action-plans/{planId}/complete.
// Requires every one of the plan's steps to already be COMPLETED. For a
// MANAGEMENT plan, completion also resolves the linked risk_escalation and
// reverts the risk from ESCALATED back to IN_REMEDIATION.
type CompleteRiskActionPlanRequest struct {
	UpdatedBy string `json:"updatedBy"`
}

// =============================================================================
// Risk Evidence File
// =============================================================================

// RiskEvidenceFile is a file uploaded as evidence for a risk's action plan or approval.
type RiskEvidenceFile struct {
	ID           int       `json:"id"`
	RiskID       int       `json:"riskId"`
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	Note         *string   `json:"note"`
	EvidenceType string    `json:"evidenceType"` // ACTION_PLAN_ATTACHMENT | FINAL_APPROVAL_ATTACHMENT
	CreatedOn    time.Time `json:"createdOn"`
}

// CreateRiskEvidenceRequest is the payload for POST /risks/{riskId}/evidence.
type CreateRiskEvidenceRequest struct {
	FileName     string  `json:"fileName"`
	FilePath     string  `json:"filePath"`
	Note         *string `json:"note"`
	EvidenceType string  `json:"evidenceType"`
	CreatedBy    string  `json:"createdBy"`
}

// ListRiskEvidenceResponse is returned by GET /risks/{riskId}/evidence.
type ListRiskEvidenceResponse struct {
	Evidence []RiskEvidenceFile `json:"evidence"`
}

// =============================================================================
// Risk Assessment
// =============================================================================

// RiskAssessment records a residual risk reassessment event.
type RiskAssessment struct {
	ID               int       `json:"id"`
	RiskID           int       `json:"riskId"`
	ScoreID          int       `json:"scoreId"`
	Progress         string    `json:"progress"`
	ReassessmentDate string    `json:"reassessmentDate"` // YYYY-MM-DD
	AssessedBy       string    `json:"assessedBy"`       // actor email
	CreatedOn        time.Time `json:"createdOn"`
	// Residual score, resolved by joining risk_score on score_id. A bare
	// scoreId is not enough for callers: the GRC backend renders the residual
	// likelihood, impact, rating, level and colour directly from this response,
	// and would otherwise have to fetch the score matrix to interpret it.
	ResidualLikelihood int    `json:"residualLikelihood"`
	ResidualImpact     int    `json:"residualImpact"`
	ResidualRating     int    `json:"residualRating"`
	ResidualLevel      string `json:"residualLevel"`
	ResidualColorCode  string `json:"residualColorCode"`
}

// CreateRiskAssessmentRequest is the payload for POST /risks/{riskId}/assessments.
//
// Likelihood and impact identify the residual score cell; the score_id is
// resolved server-side from risk_score. Callers describe the assessment they
// made, not the surrogate key of a row they would otherwise have to look up
// first.
type CreateRiskAssessmentRequest struct {
	Likelihood       int    `json:"likelihood"`
	Impact           int    `json:"impact"`
	Progress         string `json:"progress"`
	ReassessmentDate string `json:"reassessmentDate"` // YYYY-MM-DD
	AssessedBy       string `json:"assessedBy"`
	CreatedBy        string `json:"createdBy"`
}

// ListRiskAssessmentsResponse is returned by GET /risks/{riskId}/assessments.
type ListRiskAssessmentsResponse struct {
	Assessments []RiskAssessment `json:"assessments"`
}

// =============================================================================
// Audit Trail (audit_trail) — append-only
// =============================================================================

// AuditTrail is one immutable entry in the audit trail.
type AuditTrail struct {
	ID         int64     `json:"id"`
	ActorID    *int      `json:"actorId"`
	AuditID    *int      `json:"auditId"`
	ControlID  *int      `json:"controlId"`
	EvidenceID *int      `json:"evidenceId"`
	Action     string    `json:"action"`  // CREATED | UPLOADED | RESUBMITTED | APPROVED | REJECTED | COMMENTED | ESCALATED | AI_VALIDATED | EXPORTED
	Details    *string   `json:"details"` // raw JSON string
	CreatedBy  *string   `json:"createdBy"`
	CreatedOn  time.Time `json:"createdOn"`
}

// CreateAuditTrailRequest is the payload for POST /audits/{auditId}/trail.
type CreateAuditTrailRequest struct {
	ActorID    *int    `json:"actorId"`
	ControlID  *int    `json:"controlId"`
	EvidenceID *int    `json:"evidenceId"`
	Action     string  `json:"action"`
	Details    *string `json:"details"`
	CreatedBy  *string `json:"createdBy"`
}

// ListAuditTrailResponse is returned by GET /audits/{auditId}/trail.
type ListAuditTrailResponse struct {
	Trail  []AuditTrail `json:"trail"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// =============================================================================
// Risk Action Step (risk_action_step)
// =============================================================================

// RiskActionStep is one numbered step within a risk action plan.
type RiskActionStep struct {
	ID            int       `json:"id"`
	PlanID        int       `json:"planId"`
	StepNo        int       `json:"stepNo"`
	Description   *string   `json:"description"`
	Status        string    `json:"status"` // PENDING | IN_PROGRESS | COMPLETED
	CompletedDate *string   `json:"completedDate"`
	CreatedOn     time.Time `json:"createdOn"`
	UpdatedOn     time.Time `json:"updatedOn"`
}

// CreateRiskActionStepRequest is the payload for POST /action-plans/{planId}/steps.
type CreateRiskActionStepRequest struct {
	StepNo      int     `json:"stepNo"`
	Description *string `json:"description"`
	CreatedBy   string  `json:"createdBy"`
}

// UpdateRiskActionStepRequest is the payload for PATCH /action-plans/{planId}/steps/{stepId}.
type UpdateRiskActionStepRequest struct {
	Description   *string `json:"description"`
	Status        *string `json:"status"`        // PENDING | IN_PROGRESS | COMPLETED
	CompletedDate *string `json:"completedDate"` // YYYY-MM-DD
	StepNo        *int    `json:"stepNo"`
	UpdatedBy     string  `json:"updatedBy"`
}

// ListRiskActionStepsResponse is returned by GET /action-plans/{planId}/steps.
type ListRiskActionStepsResponse struct {
	Steps []RiskActionStep `json:"steps"`
}

// =============================================================================
// Risk Compliance Reference Link (risk_compliance_reference junction)
// =============================================================================

// RiskComplianceRefLink is one row in the risk_compliance_reference junction table.
// Name and Description are joined from risk_security_compliance_reference.
type RiskComplianceRefLink struct {
	RiskID      int       `json:"riskId"`
	ReferenceID int       `json:"referenceId"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedOn   time.Time `json:"createdOn"`
}

// AddRiskComplianceRefRequest is the payload for POST /risks/{riskId}/compliance-references.
type AddRiskComplianceRefRequest struct {
	ReferenceID int `json:"referenceId"`
}

// ListRiskComplianceRefsResponse is returned by GET /risks/{riskId}/compliance-references.
type ListRiskComplianceRefsResponse struct {
	References []RiskComplianceRefLink `json:"references"`
}

// =============================================================================
// Risk Escalation (risk_escalation)
// =============================================================================

// RiskEscalation records an escalation of a risk to Management. It is created
// automatically by the daily overdue-risk job (see internal/job) — there is no
// human-supplied target or reason; the trigger is always "IN_REMEDIATION past
// implementation_date".
type RiskEscalation struct {
	ID                   int       `json:"id"`
	RiskID               int       `json:"riskId"`
	NewTreatmentStrategy *string   `json:"newTreatmentStrategy"`
	ActionPlanID         *int      `json:"actionPlanId"`
	Decision             *string   `json:"decision"`
	Status               string    `json:"status"` // OPEN | RESOLVED
	CreatedBy            *string   `json:"createdBy"`
	UpdatedBy            *string   `json:"updatedBy"`
	CreatedOn            time.Time `json:"createdOn"`
	UpdatedOn            time.Time `json:"updatedOn"`
}

// CreateRiskEscalationRequest is the payload for POST /risks/{riskId}/escalations.
type CreateRiskEscalationRequest struct {
	NewTreatmentStrategy *string `json:"newTreatmentStrategy"`
	ActionPlanID         *int    `json:"actionPlanId"`
	CreatedBy            string  `json:"createdBy"`
}

// EscalateRiskRequest is the payload for POST /risks/{riskId}/escalate — the
// manual trigger a Compliance user clicks on an overdue IN_REMEDIATION risk,
// as an alternative to waiting for the daily job to reach it.
type EscalateRiskRequest struct {
	CreatedBy string `json:"createdBy"`
}

// UpdateRiskEscalationRequest is the payload for PATCH /risks/{riskId}/escalations/{escalationId}.
type UpdateRiskEscalationRequest struct {
	Decision             *string `json:"decision"`
	NewTreatmentStrategy *string `json:"newTreatmentStrategy"`
	ActionPlanID         *int    `json:"actionPlanId"`
	Status               *string `json:"status"` // OPEN | RESOLVED
	UpdatedBy            string  `json:"updatedBy"`
}

// ListRiskEscalationsResponse is returned by GET /risks/{riskId}/escalations.
type ListRiskEscalationsResponse struct {
	Escalations []RiskEscalation `json:"escalations"`
}

// =============================================================================
// Risk Change Log (risk_change_log) — append-only
// =============================================================================

// RiskChangeLog is one field-level change entry for a risk.
type RiskChangeLog struct {
	ID           int64     `json:"id"`
	RiskID       int       `json:"riskId"`
	CreatedBy    string    `json:"createdBy"`
	Action       string    `json:"action"`       // CREATE | UPDATE | DELETE
	FieldChanged *string   `json:"fieldChanged"` // nil when action is CREATE or DELETE
	OldValue     *string   `json:"oldValue"`     // raw JSON
	NewValue     *string   `json:"newValue"`     // raw JSON
	CreatedOn    time.Time `json:"createdOn"`
}

// CreateRiskChangeLogRequest is the payload for POST /risks/{riskId}/changes.
type CreateRiskChangeLogRequest struct {
	CreatedBy    string  `json:"createdBy"`
	Action       string  `json:"action"` // CREATE | UPDATE | DELETE
	FieldChanged *string `json:"fieldChanged"`
	OldValue     *string `json:"oldValue"` // raw JSON string
	NewValue     *string `json:"newValue"` // raw JSON string
}

// ListRiskChangeLogResponse is returned by GET /risks/{riskId}/changes.
type ListRiskChangeLogResponse struct {
	Changes []RiskChangeLog `json:"changes"`
	Total   int             `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
}

// =============================================================================
// Risk Notification (risk_notification)
// =============================================================================

// RiskNotification is a single in-app/email notification for a risk-module
// event. Only the write path (create) has a caller so far — the escalation
// job and the action-plan-completion cascade. There is no notification-center
// UI yet, so List/MarkRead exist as a real API surface but are unconsumed
// until that UI is built, the same way risk_change_log's read side sat unused
// before the changelog viewer existed.
type RiskNotification struct {
	ID          int64     `json:"id"`
	RecipientID int       `json:"recipientId"`
	RiskID      *int      `json:"riskId"`
	Type        string    `json:"type"`    // REMINDER | ESCALATION | STATUS_CHANGE | APPROVAL | REASSESSMENT | REJECTION
	Channel     string    `json:"channel"` // EMAIL | IN_APP
	Message     string    `json:"message"`
	IsRead      bool      `json:"isRead"`
	CreatedBy   *string   `json:"createdBy"`
	UpdatedBy   *string   `json:"updatedBy"`
	CreatedOn   time.Time `json:"createdOn"`
	UpdatedOn   time.Time `json:"updatedOn"`
}

// CreateRiskNotificationRequest is the payload for POST /notifications.
// Channel defaults to IN_APP when omitted — no email transport exists yet
// (see the email-service integration this is deliberately deferred to).
type CreateRiskNotificationRequest struct {
	RecipientID int     `json:"recipientId"`
	RiskID      *int    `json:"riskId"`
	Type        string  `json:"type"`
	Channel     *string `json:"channel"`
	Message     string  `json:"message"`
	CreatedBy   string  `json:"createdBy"`
}

// MarkRiskNotificationReadRequest is the payload for PATCH /notifications/{id}/read.
// RecipientID scopes the update so one recipient cannot mark another's notification read.
type MarkRiskNotificationReadRequest struct {
	RecipientID int    `json:"recipientId"`
	UpdatedBy   string `json:"updatedBy"`
}

// ListRiskNotificationsResponse is returned by GET /users/{userId}/notifications.
type ListRiskNotificationsResponse struct {
	Notifications []RiskNotification `json:"notifications"`
}

// =============================================================================
// Audit Comment (audit_comment) — threaded comments on an evidence submission
// =============================================================================

// AuditComment is one comment on an evidence submission. Threaded via
// ParentCommentID; IsInternal hides it from the external auditor.
type AuditComment struct {
	ID              int       `json:"id"`
	EvidenceID      int       `json:"evidenceId"`
	AuthorID        *int      `json:"authorId"`
	ParentCommentID *int      `json:"parentCommentId"`
	Content         string    `json:"content"`
	IsInternal      bool      `json:"isInternal"`
	CreatedBy       *string   `json:"createdBy"`
	CreatedOn       time.Time `json:"createdOn"`
	UpdatedOn       time.Time `json:"updatedOn"`
}

// CreateAuditCommentRequest is the payload for POST /evidence/{evidenceId}/comments.
type CreateAuditCommentRequest struct {
	AuthorID        *int   `json:"authorId"`
	ParentCommentID *int   `json:"parentCommentId"`
	Content         string `json:"content"`
	IsInternal      bool   `json:"isInternal"`
	CreatedBy       string `json:"createdBy"`
}

// ListAuditCommentsResponse is returned by GET /evidence/{evidenceId}/comments.
type ListAuditCommentsResponse struct {
	Comments []AuditComment `json:"comments"`
}

// =============================================================================
// Audit AI Validation Log (audit_ai_validation_log) — append-only
// =============================================================================

// AuditAIValidationLog is one AI validation run against an evidence submission.
// Written by the async validation agent; read by compliance as review hints.
type AuditAIValidationLog struct {
	ID              int64     `json:"id"`
	EvidenceID      int       `json:"evidenceId"`
	ControlID       int       `json:"controlId"`
	Result          string    `json:"result"`    // PASS | FAIL | UNCERTAIN | PENDING | ERROR
	GapsFound       *string   `json:"gapsFound"` // JSON array of gap objects
	Feedback        *string   `json:"feedback"`  // JSON array of submitter-facing action strings
	Summary         *string   `json:"summary"`
	ConfidenceScore *float64  `json:"confidenceScore"`
	CreatedBy       *string   `json:"createdBy"`
	CreatedOn       time.Time `json:"createdOn"`
}

// CreateAuditAIValidationLogRequest is the payload for POST /evidence/{evidenceId}/ai-validations.
type CreateAuditAIValidationLogRequest struct {
	ControlID       int      `json:"controlId"`
	Result          string   `json:"result"` // PASS | FAIL | UNCERTAIN | PENDING | ERROR
	GapsFound       *string  `json:"gapsFound"`
	Feedback        *string  `json:"feedback"`
	Summary         *string  `json:"summary"`
	ConfidenceScore *float64 `json:"confidenceScore"`
	CreatedBy       string   `json:"createdBy"`
}

// ListAuditAIValidationLogsResponse is returned by GET /evidence/{evidenceId}/ai-validations.
type ListAuditAIValidationLogsResponse struct {
	Validations []AuditAIValidationLog `json:"validations"`
}

// NextSequenceResponse is returned by GET /risks/next-sequence-number.
type NextSequenceResponse struct {
	NextSequenceNumber int `json:"nextSequenceNumber"`
}

// =============================================================================
// Risk detail
// =============================================================================

// RiskDetail is the fully-composed risk returned by GET /risks/{id}/detail:
// every risk column, the display names its foreign keys resolve to, both scores,
// and the related rows a risk page needs. Assembling it here rather than making
// the caller issue six requests keeps the read consistent — the parts cannot
// disagree with each other — and keeps a page load to one round trip.
//
// It carries only what is stored. Presentation-level entries, such as the
// synthetic "initial" assessment some callers prepend so an assessment log
// reads gross → reassessment → reassessment, are the caller's business.
type RiskDetail struct {
	Risk

	// ComplianceApproverName resolves compliance_approval_by, which the
	// summary Risk does not carry.
	ComplianceApproverName *string `json:"complianceApproverName"`

	// GrossScore is the rating given at creation. EffectiveScore is the
	// residual standing now: the most recent assessment's score when one
	// exists, otherwise the gross score. Both are nil when a risk has no
	// gross score and no assessments.
	GrossScore     *RiskScore `json:"grossScore"`
	EffectiveScore *RiskScore `json:"effectiveScore"`

	ComplianceReferences []RiskComplianceReference `json:"complianceReferences"`
	RiskCategories       []RiskCategory            `json:"riskCategories"`
	ActionPlan           *RiskActionPlanDetail     `json:"actionPlan"`
	Assessments          []RiskAssessment          `json:"assessments"`
}

// RiskActionPlanDetail is the risk's STANDARD action plan with its steps
// embedded, ordered by step_no.
type RiskActionPlanDetail struct {
	ID            int              `json:"id"`
	RiskID        int              `json:"riskId"`
	ActionOwnerID *int             `json:"actionOwnerId"`
	Description   *string          `json:"description"`
	Status        string           `json:"status"`
	PlanType      string           `json:"planType"`
	Steps         []RiskActionStep `json:"steps"`
}

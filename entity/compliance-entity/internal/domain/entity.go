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
type User struct {
	ID          int       `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	UserType    string    `json:"userType"` // INTERNAL | EXTERNAL
	AuditTeamID *int      `json:"auditTeamId"`
	RiskTeamID  *int      `json:"riskTeamId"`
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
	// AuditorEmail identifies the assigned auditor for the assigned-auditor gate
	// (population validation, sample selection, evidence validation): the caller
	// is authorized when their token email matches this value.
	AuditorEmail        *string   `json:"auditorEmail"`
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
	WorkflowStatus     string    `json:"workflowStatus"`
	TreatmentStrategy  *string   `json:"treatmentStrategy"`
	GrossScoreID       *int      `json:"grossScoreId"`
	GrossRiskLevel     *string   `json:"grossRiskLevel"`
	ImplementationDate *string   `json:"implementationDate"` // YYYY-MM-DD
	ReassessmentDate   *string   `json:"reassessmentDate"`   // YYYY-MM-DD
	CreatedOn          time.Time `json:"createdOn"`
	UpdatedOn          time.Time `json:"updatedOn"`
}

// SearchRisksRequest is the payload for POST /risks/search.
type SearchRisksRequest struct {
	SearchQuery        string     `json:"searchQuery"`
	WorkflowStatusKeys []string   `json:"workflowStatusKeys"` // filter by status values
	SourceRegisterIDs  []int      `json:"sourceRegisterIds"`
	AssignmentTeamIDs  []int      `json:"assignmentTeamIds"`
	RiskYears          []int      `json:"riskYears"`
	RiskQuarterKeys    []string   `json:"riskQuarterKeys"` // Q1 | Q2 | Q3 | Q4
	Pagination         Pagination `json:"pagination"`
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
	RiskTeamID  *int   `json:"riskTeamId"`
	Status      string `json:"status"`
	CreatedBy   string `json:"createdBy"`
}

// UpdateUserRequest is the payload for PATCH /users/{id}.
type UpdateUserRequest struct {
	DisplayName *string `json:"displayName"`
	UserType    *string `json:"userType"` // INTERNAL | EXTERNAL
	AuditTeamID *int    `json:"auditTeamId"`
	RiskTeamID  *int    `json:"riskTeamId"`
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
	RiskTitle          string  `json:"riskTitle"`
	RiskDescription    *string `json:"riskDescription"`
	SourceRegisterID   int     `json:"sourceRegisterId"`
	AssignmentTeamID   int     `json:"assignmentTeamId"`
	AssignerID         int     `json:"assignerId"`
	OwnerID            int     `json:"ownerId"`
	RiskYear           int     `json:"riskYear"`
	RiskQuarter        string  `json:"riskQuarter"` // Q1 | Q2 | Q3 | Q4
	GrossScoreID       *int    `json:"grossScoreId"`
	TreatmentStrategy  *string `json:"treatmentStrategy"`
	ImplementationDate *string `json:"implementationDate"` // YYYY-MM-DD
	ReassessmentDate   *string `json:"reassessmentDate"`   // YYYY-MM-DD
	ImpactDescription  *string `json:"impactDescription"`
	RiskIdentifiedDate *string `json:"riskIdentifiedDate"`
	IdentifiedByType   *string `json:"identifiedByType"` // EMPLOYEE | EXTERNAL_PERSON | TOOL
	IdentifiedByUserID *int    `json:"identifiedByUserId"`
	IdentifiedByName   *string `json:"identifiedByName"`
	GitIssueURL        *string `json:"gitIssueUrl"`
	EmailSubject       *string `json:"emailSubject"`
	Remarks            *string `json:"remarks"`
	CreatedBy          string  `json:"createdBy"`
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
	ActionPlanID           *int    `json:"actionPlanId"`
	GitIssueURL            *string `json:"gitIssueUrl"`
	Remarks                *string `json:"remarks"`
	UpdatedBy              string  `json:"updatedBy"`
	ExpectedStatus         string  `json:"-"` // set server-side for atomic transition; never decoded from JSON
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
	CreatedOn     time.Time `json:"createdOn"`
	UpdatedOn     time.Time `json:"updatedOn"`
}

// CreateRiskActionPlanRequest is the payload for POST /risks/{riskId}/action-plans.
type CreateRiskActionPlanRequest struct {
	Description   *string `json:"description"`
	ActionOwnerID *int    `json:"actionOwnerId"`
	PlanType      string  `json:"planType"` // STANDARD | MANAGEMENT
	CreatedBy     string  `json:"createdBy"`
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
}

// CreateRiskAssessmentRequest is the payload for POST /risks/{riskId}/assessments.
type CreateRiskAssessmentRequest struct {
	ScoreID          int    `json:"scoreId"`
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

// TrailFilter narrows a GET /audits/{auditId}/trail listing. ControlIDs empty
// means "don't filter on this"; multiple values are OR'd (IN (...)), matching
// the audit-wide activity log's Control column filter. Empty returns the whole
// audit's trail (audit-level rows and every control's rows together).
type TrailFilter struct {
	ControlIDs []int
	From       *time.Time
	To         *time.Time
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

// RiskEscalation records an escalation of a risk to Management.
type RiskEscalation struct {
	ID                   int       `json:"id"`
	RiskID               int       `json:"riskId"`
	EscalatedTo          int       `json:"escalatedTo"`
	Reason               *string   `json:"reason"`
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
	EscalatedTo          int     `json:"escalatedTo"`
	Reason               *string `json:"reason"`
	NewTreatmentStrategy *string `json:"newTreatmentStrategy"`
	ActionPlanID         *int    `json:"actionPlanId"`
	CreatedBy            string  `json:"createdBy"`
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

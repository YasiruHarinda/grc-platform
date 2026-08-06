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

// AuditStats are audit-level counts shown on the dashboard.
type AuditStats struct {
	TotalAudits     int `json:"totalAudits"`
	ActiveAudits    int `json:"activeAudits"`
	CompletedAudits int `json:"completedAudits"`
	ArchivedAudits  int `json:"archivedAudits"`
}

// DashboardStats are the top-level summary numbers on the dashboard.
type DashboardStats struct {
	TotalControls            int     `json:"totalControls"`
	CompletedControls        int     `json:"completedControls"`
	OverdueControls          int     `json:"overdueControls"`
	EvidenceRequiredControls int     `json:"evidenceRequiredControls"`
	CompletionPercent        float64 `json:"completionPercent"`
	TotalActionItems         int     `json:"totalActionItems"`
}

// StatusCount is one slice of the "Controls by Status" donut chart.
type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// TeamCompletion is one bar in the "Completed by Team" chart.
type TeamCompletion struct {
	Team      string `json:"team"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Overdue   int    `json:"overdue"`
}

// TeamStatusCount is one team's control count for a single status — feeds the
// per-team status breakdown in the dashboard's team drill-down.
type TeamStatusCount struct {
	Team   string `json:"team"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// ActionItem is a single entry in the "My Action Items" list.
type ActionItem struct {
	ControlID     int    `json:"controlId"`
	AuditID       int    `json:"auditId"`
	AuditName     string `json:"auditName"`
	ControlNumber string `json:"controlNumber"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	DueDate       string `json:"dueDate"`
	Team          string `json:"team"`
	ProcessOwner  string `json:"processOwner"`
	TeamID        *int   `json:"teamId"`
	OwnerID       *int   `json:"ownerId"`
}

// OverdueControl is a single entry in the "Overdue Controls" list.
type OverdueControl struct {
	ControlID     int    `json:"controlId"`
	AuditID       int    `json:"auditId"`
	AuditName     string `json:"auditName"`
	ControlNumber string `json:"controlNumber"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	DueDate       string `json:"dueDate"`
	Team          string `json:"team"`
	ProcessOwner  string `json:"processOwner"`
	TeamID        *int   `json:"teamId"`
	OwnerID       *int   `json:"ownerId"`
}

// DashboardData is the full payload returned by GET /api/v1/audit/dashboard.
type DashboardData struct {
	AuditStats             AuditStats        `json:"auditStats"`
	Stats                  DashboardStats    `json:"stats"`
	StatusDistribution     []StatusCount     `json:"statusDistribution"`
	TeamCompletion         []TeamCompletion  `json:"teamCompletion"`
	TeamStatusDistribution []TeamStatusCount `json:"teamStatusDistribution"`
	ActionItems            []ActionItem      `json:"actionItems"`
	DueSoonItems           []ActionItem      `json:"dueSoonItems"`
	OverdueControls        []OverdueControl  `json:"overdueControls"`
}

// WorkQueueTab identifies which sub-list the caller wants.
type WorkQueueTab string

const (
	WorkQueueTabActionItems WorkQueueTab = "action-items"
	WorkQueueTabDueSoon     WorkQueueTab = "due-soon"
	WorkQueueTabOverdue     WorkQueueTab = "overdue"
)

// WorkQueuePage is the paginated response for GET /api/v1/audit/work-queue.
type WorkQueuePage struct {
	Items []ActionItem `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Limit int          `json:"limit"`
}

// DashboardFilter carries the query scope derived from the user's JWT roles.
type DashboardFilter struct {
	// Roles is the set of role strings from the JWT groups claim.
	Roles []string
	// UserEmail is the authenticated user's email (used to look up team/auditor ID).
	UserEmail string
	// TeamIDs optionally restricts work-queue results to specific audit_team IDs.
	TeamIDs []int
	// OwnerIDs optionally restricts work-queue results to specific process owner user IDs.
	OwnerIDs []int
	// AuditIDs optionally restricts work-queue results to specific audit IDs.
	AuditIDs []int
	// ControlNumber optionally restricts results to controls whose number contains
	// this (case-insensitive) substring; "" = no filter.
	ControlNumber string
	// Statuses optionally restricts results to specific control statuses; nil/empty =
	// all. The webapp folds both the status and action-needed column filters into this.
	Statuses []string
	// DueSortDesc sorts the page by due date descending (latest first) when true.
	DueSortDesc bool
}

// Role constants — mirror the Asgardeo group names exactly.
const (
	RoleComplianceAdmin = "compliance_admin"
	RoleComplianceTeam  = "compliance_team"
	RoleInternalTeam    = "internal_team"
	RoleExternalAuditor = "external_auditor"
	RoleManagement      = "management"
)

// asgardeoGroupRoles maps Asgardeo group names (JWT groups claim) to the
// canonical dashboard role tokens the Compliance Entity understands. The
// entity scopes unknown roles to zero rows, so translation must happen here
// (the entity itself is owned by another team and cannot be changed).
var asgardeoGroupRoles = map[string]string{
	"grc-platform-compliance-audit-admin": RoleComplianceAdmin,
	"grc-platform-compliance-audit-team":  RoleComplianceTeam,
	"grc-platform-internal-team":          RoleInternalTeam,
	"grc-platform-external-auditor":       RoleExternalAuditor,
	"grc-platform-management":             RoleManagement,
	// Read-only auditor sees the same org-wide scope as management.
	"grc-platform-internal-auditor": RoleManagement,
	// Testing catch-all group — full visibility for local development.
	"wso2-everyone": RoleComplianceAdmin,
}

// NormalizedRoles returns Roles translated to canonical role tokens.
// Values that are already canonical (or unknown) pass through unchanged.
func (f DashboardFilter) NormalizedRoles() []string {
	out := make([]string, 0, len(f.Roles))
	for _, g := range f.Roles {
		if r, ok := asgardeoGroupRoles[g]; ok {
			out = append(out, r)
			continue
		}
		out = append(out, g)
	}
	return out
}

// PrimaryRole returns the highest-priority audit role from the filter's role list.
func (f DashboardFilter) PrimaryRole() string {
	priority := []string{
		RoleComplianceAdmin,
		RoleComplianceTeam,
		RoleExternalAuditor,
		RoleInternalTeam,
		RoleManagement,
	}
	for _, r := range priority {
		for _, g := range f.Roles {
			if g == r {
				return r
			}
		}
	}
	return ""
}

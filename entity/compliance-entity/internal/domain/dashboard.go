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

package domain

// AuditDashboardRequest is the body of POST /audit/dashboard. Roles come from the
// caller's JWT groups claim; UserEmail is the authenticated user (for team/auditor scope).
type AuditDashboardRequest struct {
	Roles     []string `json:"roles"`
	UserEmail string   `json:"userEmail"`
}

// Audit role constants — mirror the Asgardeo group names exactly.
const (
	RoleComplianceAdmin = "compliance_admin"
	RoleComplianceTeam  = "compliance_team"
	RoleInternalTeam    = "internal_team"
	RoleExternalAuditor = "external_auditor"
	RoleManagement      = "management"
)

// PrimaryRole returns the highest-priority audit role from the request's role list.
func (req AuditDashboardRequest) PrimaryRole() string {
	priority := []string{RoleComplianceAdmin, RoleComplianceTeam, RoleExternalAuditor, RoleInternalTeam, RoleManagement}
	for _, r := range priority {
		for _, g := range req.Roles {
			if g == r {
				return r
			}
		}
	}
	return ""
}

// AuditStats are the audit-count summary tiles.
type AuditStats struct {
	TotalAudits     int `json:"totalAudits"`
	ActiveAudits    int `json:"activeAudits"`
	CompletedAudits int `json:"completedAudits"`
	ArchivedAudits  int `json:"archivedAudits"`
}

// DashboardStats are the top-level control summary numbers.
type DashboardStats struct {
	TotalControls            int     `json:"totalControls"`
	CompletedControls        int     `json:"completedControls"`
	OverdueControls          int     `json:"overdueControls"`
	EvidenceRequiredControls int     `json:"evidenceRequiredControls"`
	CompletionPercent        float64 `json:"completionPercent"`
	TotalActionItems         int     `json:"totalActionItems"`
}

// StatusCount is one slice of the "Controls by Status" chart.
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

// DashboardControlItem is a single control entry used in both "My Action Items"
// and "Overdue Controls" dashboard lists. The two lists share the same shape.
type DashboardControlItem struct {
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

// WorkQueueTab identifies which sub-list the caller wants.
type WorkQueueTab string

const (
	WorkQueueTabActionItems WorkQueueTab = "action-items"
	WorkQueueTabDueSoon     WorkQueueTab = "due-soon"
	WorkQueueTabOverdue     WorkQueueTab = "overdue"
)

// WorkQueueRequest is the body of POST /audit/work-queue/search.
type WorkQueueRequest struct {
	Roles     []string     `json:"roles"`
	UserEmail string       `json:"userEmail"`
	Tab       WorkQueueTab `json:"tab"`
	Page      int          `json:"page"`    // 1-based
	Limit     int          `json:"limit"`   // rows per page; capped at 100 server-side
	TeamIDs   []int        `json:"teamIds"` // filter by audit_team.id; nil/empty = all teams
	OwnerIDs  []int        `json:"ownerIds"` // filter by user.id (process owner); nil/empty = all owners
	AuditIDs  []int        `json:"auditIds"` // filter by audit.id; nil/empty = all audits
	// ControlNumber is a case-insensitive substring match on audit_control.control_number; "" = no filter.
	ControlNumber string `json:"controlNumber"`
	// Statuses filters by audit_control.status; nil/empty = all statuses. The backend
	// resolves both the "status" and "action needed" UI filters into this single set.
	Statuses []string `json:"statuses"`
	// DueSortDesc sorts by due date descending (latest first) when true; ascending otherwise.
	DueSortDesc bool `json:"dueSortDesc"`
}

// PrimaryRole re-uses the same priority logic as AuditDashboardRequest.
func (r WorkQueueRequest) PrimaryRole() string {
	priority := []string{RoleComplianceAdmin, RoleComplianceTeam, RoleExternalAuditor, RoleInternalTeam, RoleManagement}
	for _, p := range priority {
		for _, g := range r.Roles {
			if g == p {
				return p
			}
		}
	}
	return ""
}

// WorkQueuePage is the paginated response for POST /audit/work-queue/search.
type WorkQueuePage struct {
	Items []DashboardControlItem `json:"items"`
	Total int                    `json:"total"`
	Page  int                    `json:"page"`
	Limit int                    `json:"limit"`
}

// DashboardData is the full audit dashboard payload.
type DashboardData struct {
	AuditStats             AuditStats             `json:"auditStats"`
	Stats                  DashboardStats         `json:"stats"`
	StatusDistribution     []StatusCount          `json:"statusDistribution"`
	TeamCompletion         []TeamCompletion       `json:"teamCompletion"`
	TeamStatusDistribution []TeamStatusCount      `json:"teamStatusDistribution"`
	ActionItems            []DashboardControlItem `json:"actionItems"`
	DueSoonItems           []DashboardControlItem `json:"dueSoonItems"`
	OverdueControls        []DashboardControlItem `json:"overdueControls"`
}

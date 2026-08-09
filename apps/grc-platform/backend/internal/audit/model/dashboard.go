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
	PendingItems           []ActionItem      `json:"pendingItems"`
	ValidationItems        []ActionItem      `json:"validationItems"`
	OverdueControls        []OverdueControl  `json:"overdueControls"`
}

// WorkQueueTab identifies which sub-list the caller wants.
type WorkQueueTab string

const (
	WorkQueueTabActionItems WorkQueueTab = "action-items"
	WorkQueueTabDueSoon     WorkQueueTab = "due-soon"
	WorkQueueTabPending     WorkQueueTab = "pending"
	WorkQueueTabValidation  WorkQueueTab = "validation"
	WorkQueueTabOverdue     WorkQueueTab = "overdue"
)

// WorkQueuePage is the paginated response for GET /api/v1/audit/work-queue.
type WorkQueuePage struct {
	Items []ActionItem `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Limit int          `json:"limit"`
}

// Scope is the row-visibility class the Compliance Entity applies. It is derived
// from the actor's privileges in the handler (never from a role or group name)
// and sent to the entity explicitly. See docs/adr/0002-privilege-derived-scope.md.
type Scope string

const (
	// ScopeAll sees every row (org-wide read — holders of AUDIT_VIEW_ALL_AUDITS).
	ScopeAll Scope = "all"
	// ScopeOwnTeam sees the actor's audit teams' controls (submitter dashboards).
	ScopeOwnTeam Scope = "own_team"
	// ScopeOwned sees only controls the actor owns (owner_id = actor) — the
	// submitter's work queue, narrower than their own_team dashboard.
	ScopeOwned Scope = "owned"
	// ScopeAssigned sees only controls the actor audits (auditor_id = actor).
	ScopeAssigned Scope = "assigned"
	// ScopeNone sees nothing.
	ScopeNone Scope = "none"
)

// WorkQueueClass selects which control-lifecycle bucket counts as the actor's
// "action items", derived from privileges (never a role). Sent to the entity so
// it needs no role knowledge to compute the action-items list/count.
type WorkQueueClass string

const (
	// WorkQueueClassReview — internal review stage (holders of AUDIT_REVIEW_EVIDENCE).
	WorkQueueClassReview WorkQueueClass = "review"
	// WorkQueueClassSubmission — submission stage (submitters without review).
	WorkQueueClassSubmission WorkQueueClass = "submission"
	// WorkQueueClassValidation — auditor validation/sample stage.
	WorkQueueClassValidation WorkQueueClass = "validation"
	// WorkQueueClassNone — no action queue (e.g. management observers).
	WorkQueueClassNone WorkQueueClass = "none"
)

// DashboardFilter carries the query scope, derived from the user's privileges.
type DashboardFilter struct {
	// ViewScope scopes dashboard stats, charts and lists.
	ViewScope Scope
	// WorkQueueScope scopes the work-queue tabs. Equals ViewScope for every role
	// except the submitter (own_team view, owned work queue).
	WorkQueueScope Scope
	// WorkQueueClass selects the action-items lifecycle bucket for the actor.
	WorkQueueClass WorkQueueClass
	// UserEmail is the authenticated user's email (used to look up team/owner/auditor ID).
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

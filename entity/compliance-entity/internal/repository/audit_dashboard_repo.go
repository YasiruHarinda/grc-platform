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

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// DashboardRepository aggregates the audit dashboard from the audit tables.
type DashboardRepository interface {
	Get(ctx context.Context, req domain.AuditDashboardRequest) (*domain.DashboardData, error)
	GetWorkQueuePage(ctx context.Context, req domain.WorkQueueRequest) (*domain.WorkQueuePage, error)
}

type dashboardRepo struct{ db *sql.DB }

// NewDashboardRepository constructs a DashboardRepository.
func NewDashboardRepository(db *sql.DB) DashboardRepository { return &dashboardRepo{db: db} }

// scopeWhere returns a WHERE fragment (starting with "AND"), any args to bind,
// and an error, for the given row scope. Scope is derived from the caller's
// privileges upstream (never a role); this just applies it, resolving the actor's
// team/owner/auditor identity from userEmail. Only sql.ErrNoRows / a NULL user is
// mapped to " AND 1=0" (a legitimate no-data case); any other DB error propagates
// so callers return 500 instead of a silent empty dashboard.
func (r *dashboardRepo) scopeWhere(ctx context.Context, scope domain.Scope, userEmail string) (string, []any, error) {
	switch scope {
	case domain.ScopeAll:
		return "", nil, nil
	case domain.ScopeOwned:
		uid, ok, err := r.userIDByEmail(ctx, userEmail)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			return " AND 1=0", nil, nil
		}
		return " AND c.owner_id = ?", []any{uid}, nil
	case domain.ScopeAssigned:
		uid, ok, err := r.userIDByEmail(ctx, userEmail)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			return " AND 1=0", nil, nil
		}
		return " AND c.auditor_id = ?", []any{uid}, nil
	default: // ScopeNone and any unrecognized value scope to nothing.
		return " AND 1=0", nil, nil
	}
}

// userIDByEmail resolves a user's id from their email. ok=false when no such user
// (or a NULL id) exists — a legitimate "scope to zero rows" case, not an error.
func (r *dashboardRepo) userIDByEmail(ctx context.Context, email string) (int64, bool, error) {
	var id sql.NullInt64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM `user` WHERE email = ?", email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !id.Valid) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("dashboard.userIDByEmail %q: %w", email, err)
	}
	return id.Int64, true, nil
}

func (r *dashboardRepo) Get(ctx context.Context, req domain.AuditDashboardRequest) (*domain.DashboardData, error) {
	// Two scopes: the view scope drives stats/charts (baseWhere); the work-queue
	// scope drives the action/due/pending/validation/overdue lists (queueWhere).
	scope, args, err := r.scopeWhere(ctx, req.Scope, req.UserEmail)
	if err != nil {
		return nil, err
	}
	baseWhere := "WHERE a.status = 'ACTIVE'" + scope

	queueScope, queueArgs, err := r.scopeWhere(ctx, req.WorkQueueScope, req.UserEmail)
	if err != nil {
		return nil, err
	}
	queueWhere := "WHERE a.status = 'ACTIVE'" + queueScope

	// Status distribution.
	statusRows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT c.status, COUNT(*) FROM audit_control c
		JOIN audit a ON a.id = c.audit_id %s GROUP BY c.status`, baseWhere), args...) // #nosec G201
	if err != nil {
		return nil, err
	}
	defer statusRows.Close()
	statusDist := []domain.StatusCount{}
	totalControls, completedControls := 0, 0
	for statusRows.Next() {
		var sc domain.StatusCount
		if err := statusRows.Scan(&sc.Status, &sc.Count); err != nil {
			return nil, err
		}
		statusDist = append(statusDist, sc)
		totalControls += sc.Count
		if sc.Status == "COMPLETE" {
			completedControls = sc.Count
		}
	}
	if err := statusRows.Err(); err != nil {
		return nil, err
	}

	// Team completion.
	teamRows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT COALESCE(t.name,'Unassigned'), COUNT(*), SUM(c.status='COMPLETE'),
		       SUM(c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE')
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id %s
		GROUP BY c.team_id, t.name ORDER BY COUNT(*) DESC LIMIT 10`, baseWhere), args...) // #nosec G201
	if err != nil {
		return nil, err
	}
	defer teamRows.Close()
	teamCompletion := []domain.TeamCompletion{}
	for teamRows.Next() {
		var tc domain.TeamCompletion
		if err := teamRows.Scan(&tc.Team, &tc.Total, &tc.Completed, &tc.Overdue); err != nil {
			return nil, err
		}
		teamCompletion = append(teamCompletion, tc)
	}
	if err := teamRows.Err(); err != nil {
		return nil, err
	}

	// Per-team status breakdown — feeds the dashboard's team drill-down.
	teamStatusRows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT COALESCE(t.name,'Unassigned'), c.status, COUNT(*)
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id %s
		GROUP BY c.team_id, t.name, c.status`, baseWhere), args...) // #nosec G201
	if err != nil {
		return nil, err
	}
	defer teamStatusRows.Close()
	teamStatusDist := []domain.TeamStatusCount{}
	for teamStatusRows.Next() {
		var ts domain.TeamStatusCount
		if err := teamStatusRows.Scan(&ts.Team, &ts.Status, &ts.Count); err != nil {
			return nil, err
		}
		teamStatusDist = append(teamStatusDist, ts)
	}
	if err := teamStatusRows.Err(); err != nil {
		return nil, err
	}

	// Overdue + evidence-required counts.
	var overdueCount, evidenceReqCount int
	if err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s
		AND c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE'`, baseWhere),
		args...).Scan(&overdueCount); err != nil { // #nosec G201
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s
		AND c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION')`, baseWhere),
		args...).Scan(&evidenceReqCount); err != nil { // #nosec G201
		return nil, err
	}

	auditStats, err := r.queryAuditStats(ctx)
	if err != nil {
		return nil, err
	}
	actionItems, err := r.queryActionItems(ctx, req.WorkQueueClass, queueWhere, queueArgs)
	if err != nil {
		return nil, err
	}
	dueSoonItems, err := r.queryDueSoonItems(ctx, queueWhere, queueArgs)
	if err != nil {
		return nil, err
	}
	pendingItems, err := r.queryStatusItems(ctx, queueWhere, queueArgs, pendingStatusFilter, 500)
	if err != nil {
		return nil, err
	}
	validationItems, err := r.queryStatusItems(ctx, queueWhere, queueArgs, validationStatusFilter, 500)
	if err != nil {
		return nil, err
	}
	totalActionItems, err := r.queryActionItemsCount(ctx, req.WorkQueueClass, queueWhere, queueArgs)
	if err != nil {
		return nil, err
	}
	overdueControls, err := r.queryOverdueControls(ctx, queueWhere, queueArgs)
	if err != nil {
		return nil, err
	}

	completionPct := 0.0
	if totalControls > 0 {
		completionPct = float64(completedControls) / float64(totalControls) * 100
	}
	return &domain.DashboardData{
		AuditStats: auditStats,
		Stats: domain.DashboardStats{
			TotalControls: totalControls, CompletedControls: completedControls,
			OverdueControls: overdueCount, EvidenceRequiredControls: evidenceReqCount,
			CompletionPercent: completionPct, TotalActionItems: totalActionItems,
		},
		StatusDistribution:     statusDist,
		TeamCompletion:         teamCompletion,
		TeamStatusDistribution: teamStatusDist,
		ActionItems:            actionItems,
		DueSoonItems:           dueSoonItems,
		PendingItems:           pendingItems,
		ValidationItems:        validationItems,
		OverdueControls:        overdueControls,
	}, nil
}

func (r *dashboardRepo) queryAuditStats(ctx context.Context) (domain.AuditStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM audit WHERE status IN ('ACTIVE','COMPLETED','ARCHIVED') GROUP BY status`)
	if err != nil {
		return domain.AuditStats{}, err
	}
	defer rows.Close()
	var s domain.AuditStats
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return domain.AuditStats{}, err
		}
		s.TotalAudits += cnt
		switch status {
		case "ACTIVE":
			s.ActiveAudits = cnt
		case "COMPLETED":
			s.CompletedAudits = cnt
		case "ARCHIVED":
			s.ArchivedAudits = cnt
		}
	}
	return s, rows.Err()
}

// actionItemsStatusFilter maps the caller-supplied work-queue class to the
// control statuses that count as that actor's action items. ok=false means the
// actor has no action queue (e.g. management), so callers return an empty list.
func (r *dashboardRepo) actionItemsStatusFilter(class domain.WorkQueueClass) (string, bool) {
	switch class {
	case domain.WorkQueueClassSubmission:
		return "c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION','POPULATION_PENDING','POPULATION_NEED_CLARIFICATION')", true
	case domain.WorkQueueClassReview:
		return "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')", true
	case domain.WorkQueueClassValidation:
		return "c.status IN ('EVIDENCE_UNDER_VALIDATION','POPULATION_UNDER_VALIDATION','POPULATION_COMPLETE','AWAITING_SAMPLE')", true
	default: // WorkQueueClassNone and any unrecognized value: no action queue.
		return "", false
	}
}

func (r *dashboardRepo) queryActionItems(ctx context.Context, class domain.WorkQueueClass, baseWhere string, scopeArgs []any) ([]domain.DashboardControlItem, error) {
	statusFilter, ok := r.actionItemsStatusFilter(class)
	if !ok {
		return []domain.DashboardControlItem{}, nil
	}
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name,
		       c.control_number,
		       c.description,
		       c.status,
		       COALESCE(DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),''),
		       COALESCE(t.name,''),
		       COALESCE(u.display_name,''),
		       c.team_id, c.owner_id
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id
		LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
		%s AND %s ORDER BY c.due_date ASC, c.id ASC LIMIT 100`, baseWhere, statusFilter) // #nosec G201
	return r.scanControlItems(ctx, q, scopeArgs)
}

// queryDueSoonItems returns controls due within 7 days in any non-terminal
// status — "due soon" is a date concern, not tied to who currently owns the
// next action, so (unlike queryActionItems) it is not filtered by role/status.
func (r *dashboardRepo) queryDueSoonItems(ctx context.Context, baseWhere string, scopeArgs []any) ([]domain.DashboardControlItem, error) {
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name,
		       c.control_number,
		       c.description,
		       c.status,
		       COALESCE(DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),''),
		       COALESCE(t.name,''),
		       COALESCE(u.display_name,''),
		       c.team_id, c.owner_id
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id
		LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
		%s AND c.status != 'COMPLETE' AND c.due_date IS NOT NULL
		AND c.due_date BETWEEN CURDATE() AND DATE_ADD(CURDATE(), INTERVAL 7 DAY)
		ORDER BY c.due_date ASC, c.id ASC LIMIT 500`, baseWhere) // #nosec G201
	return r.scanControlItems(ctx, q, scopeArgs)
}

// pendingStatusFilter matches controls whose evidence or population submission
// is awaiting the process owner (not yet submitted, kicked back for clarification,
// or a sample has been requested and evidence for it is still owed).
const pendingStatusFilter = "c.status IN ('EVIDENCE_PENDING','POPULATION_PENDING','POPULATION_NEED_CLARIFICATION','EVIDENCE_NEED_CLARIFICATION','SUBMITTED_SAMPLE')"

// validationStatusFilter matches controls whose evidence or population has been
// submitted and is now with the external auditor for validation/sampling.
const validationStatusFilter = "c.status IN ('EVIDENCE_UNDER_VALIDATION','POPULATION_UNDER_VALIDATION','POPULATION_COMPLETE','AWAITING_SAMPLE')"

// queryStatusItems returns every in-scope control matching statusFilter, most
// urgent (earliest due date) first, capped at limit. Used for the Pending and
// Under Validation work-queue lists, which — unlike Action Items — show the
// fixed status set to every role rather than a role-specific subset.
func (r *dashboardRepo) queryStatusItems(ctx context.Context, baseWhere string, scopeArgs []any, statusFilter string, limit int) ([]domain.DashboardControlItem, error) {
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name,
		       c.control_number,
		       c.description,
		       c.status,
		       COALESCE(DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),''),
		       COALESCE(t.name,''),
		       COALESCE(u.display_name,''),
		       c.team_id, c.owner_id
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id
		LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
		%s AND %s
		ORDER BY c.due_date ASC, c.id ASC LIMIT %d`, baseWhere, statusFilter, limit) // #nosec G201
	return r.scanControlItems(ctx, q, scopeArgs)
}

func (r *dashboardRepo) queryActionItemsCount(ctx context.Context, class domain.WorkQueueClass, baseWhere string, scopeArgs []any) (int, error) {
	statusFilter, ok := r.actionItemsStatusFilter(class)
	if !ok {
		return 0, nil
	}
	q := fmt.Sprintf(`
		SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id
		%s AND %s`, baseWhere, statusFilter) // #nosec G201
	var count int
	if err := r.db.QueryRowContext(ctx, q, scopeArgs...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// buildIntInFilter builds a SQL "AND col IN (?,?,...)" fragment and its args for a
// slice of int IDs. Returns empty string and nil when ids is empty.
func buildIntInFilter(col string, ids []int) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	phs := strings.Repeat("?,", len(ids))
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	return fmt.Sprintf(" AND %s IN (%s)", col, phs[:len(phs)-1]), args // #nosec G201
}

func buildStringInFilter(col string, vals []string) (string, []any) {
	if len(vals) == 0 {
		return "", nil
	}
	phs := strings.Repeat("?,", len(vals))
	args := make([]any, len(vals))
	for i, v := range vals {
		args[i] = v
	}
	return fmt.Sprintf(" AND %s IN (%s)", col, phs[:len(phs)-1]), args // #nosec G201
}

// buildLikeFilter matches col against a case-insensitive substring. An empty or
// whitespace-only term yields no filter.
func buildLikeFilter(col, term string) (string, []any) {
	term = strings.TrimSpace(term)
	if term == "" {
		return "", nil
	}
	// Escape LIKE wildcards so a literal % or _ in the term is matched literally.
	esc := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(term)
	return fmt.Sprintf(" AND %s LIKE ?", col), []any{"%" + esc + "%"} // #nosec G201
}

// GetWorkQueuePage returns a single paginated page of work-queue items.
func (r *dashboardRepo) GetWorkQueuePage(ctx context.Context, req domain.WorkQueueRequest) (*domain.WorkQueuePage, error) {
	// The work queue uses the work-queue scope (personal for submitters).
	scope, args, err := r.scopeWhere(ctx, req.WorkQueueScope, req.UserEmail)
	if err != nil {
		return nil, err
	}
	baseWhere := "WHERE a.status = 'ACTIVE'" + scope

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	// Build optional filter fragments on columns of audit_control (plus audit_id),
	// so count queries need no extra JOINs (team_id, owner_id, audit_id, status and
	// control_number are all on the base table).
	teamSQL, teamArgs := buildIntInFilter("c.team_id", req.TeamIDs)
	ownerSQL, ownerArgs := buildIntInFilter("c.owner_id", req.OwnerIDs)
	auditSQL, auditArgs := buildIntInFilter("c.audit_id", req.AuditIDs)
	statusSQL, statusArgs := buildStringInFilter("c.status", req.Statuses)
	ctrlSQL, ctrlArgs := buildLikeFilter("c.control_number", req.ControlNumber)
	filterSQL := teamSQL + ownerSQL + auditSQL + statusSQL + ctrlSQL
	filterArgs := make([]any, 0, len(teamArgs)+len(ownerArgs)+len(auditArgs)+len(statusArgs)+len(ctrlArgs))
	filterArgs = append(filterArgs, teamArgs...)
	filterArgs = append(filterArgs, ownerArgs...)
	filterArgs = append(filterArgs, auditArgs...)
	filterArgs = append(filterArgs, statusArgs...)
	filterArgs = append(filterArgs, ctrlArgs...)

	// Due-date sort direction. Controlled string (never user input), safe to inline.
	dueDir := "ASC"
	if req.DueSortDesc {
		dueDir = "DESC"
	}

	var items []domain.DashboardControlItem
	var total int

	switch req.Tab {
	case domain.WorkQueueTabActionItems:
		statusFilter, ok := r.actionItemsStatusFilter(req.WorkQueueClass)
		if !ok {
			return &domain.WorkQueuePage{Items: []domain.DashboardControlItem{}, Total: 0, Page: page, Limit: limit}, nil
		}
		// count — c.team_id and c.owner_id are on audit_control; no extra JOINs needed
		cq := fmt.Sprintf(`SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s AND %s%s`, baseWhere, statusFilter, filterSQL) // #nosec G201
		cqArgs := append(args, filterArgs...)
		if err := r.db.QueryRowContext(ctx, cq, cqArgs...).Scan(&total); err != nil {
			return nil, err
		}
		// page
		q := fmt.Sprintf(`
			SELECT c.id, c.audit_id, a.name,
			       c.control_number,
			       c.description,
			       c.status,
			       COALESCE(DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),''),
			       COALESCE(t.name,''),
			       COALESCE(u.display_name,''),
			       c.team_id, c.owner_id
			FROM audit_control c JOIN audit a ON a.id = c.audit_id
			LEFT JOIN audit_team t ON t.id = c.team_id
			LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
			%s AND %s%s ORDER BY c.due_date %s, c.id ASC LIMIT ? OFFSET ?`, baseWhere, statusFilter, filterSQL, dueDir) // #nosec G201
		pageArgs := append(append(args, filterArgs...), limit, offset)
		items, err = r.scanControlItems(ctx, q, pageArgs)

	case domain.WorkQueueTabDueSoon:
		// Due soon is a date concern, not a role/action concern — every non-terminal
		// status is included, matching queryDueSoonItems above.
		dueSoonWhere := fmt.Sprintf(`%s AND c.status != 'COMPLETE' AND c.due_date IS NOT NULL AND c.due_date BETWEEN CURDATE() AND DATE_ADD(CURDATE(), INTERVAL 7 DAY)%s`, baseWhere, filterSQL) // #nosec G201
		cq := fmt.Sprintf(`SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s`, dueSoonWhere)                                                                              // #nosec G201
		cqArgs := append(args, filterArgs...)
		if err := r.db.QueryRowContext(ctx, cq, cqArgs...).Scan(&total); err != nil {
			return nil, err
		}
		q := fmt.Sprintf(`
			SELECT c.id, c.audit_id, a.name,
			       c.control_number,
			       c.description,
			       c.status,
			       COALESCE(DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),''),
			       COALESCE(t.name,''),
			       COALESCE(u.display_name,''),
			       c.team_id, c.owner_id
			FROM audit_control c JOIN audit a ON a.id = c.audit_id
			LEFT JOIN audit_team t ON t.id = c.team_id
			LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
			%s ORDER BY c.due_date %s, c.id ASC LIMIT ? OFFSET ?`, dueSoonWhere, dueDir) // #nosec G201
		pageArgs := append(append(args, filterArgs...), limit, offset)
		items, err = r.scanControlItems(ctx, q, pageArgs)

	case domain.WorkQueueTabPending, domain.WorkQueueTabValidation:
		// Both are fixed status-set lists shown to every role the same way
		// (unlike Action Items, which is role-scoped) — only the status set differs.
		statusFilter := pendingStatusFilter
		if req.Tab == domain.WorkQueueTabValidation {
			statusFilter = validationStatusFilter
		}
		statusWhere := fmt.Sprintf(`%s AND %s%s`, baseWhere, statusFilter, filterSQL) // #nosec G201
		cq := fmt.Sprintf(`SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s`, statusWhere) // #nosec G201
		cqArgs := append(args, filterArgs...)
		if err := r.db.QueryRowContext(ctx, cq, cqArgs...).Scan(&total); err != nil {
			return nil, err
		}
		q := fmt.Sprintf(`
			SELECT c.id, c.audit_id, a.name,
			       c.control_number,
			       c.description,
			       c.status,
			       COALESCE(DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),''),
			       COALESCE(t.name,''),
			       COALESCE(u.display_name,''),
			       c.team_id, c.owner_id
			FROM audit_control c JOIN audit a ON a.id = c.audit_id
			LEFT JOIN audit_team t ON t.id = c.team_id
			LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
			%s ORDER BY c.due_date %s, c.id ASC LIMIT ? OFFSET ?`, statusWhere, dueDir) // #nosec G201
		pageArgs := append(append(args, filterArgs...), limit, offset)
		items, err = r.scanControlItems(ctx, q, pageArgs)

	default: // overdue
		overdueWhere := fmt.Sprintf(`%s AND c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE'%s`, baseWhere, filterSQL) // #nosec G201
		cq := fmt.Sprintf(`SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s`, overdueWhere)                              // #nosec G201
		cqArgs := append(args, filterArgs...)
		if err := r.db.QueryRowContext(ctx, cq, cqArgs...).Scan(&total); err != nil {
			return nil, err
		}
		q := fmt.Sprintf(`
			SELECT c.id, c.audit_id, a.name,
			       c.control_number,
			       c.description,
			       c.status,
			       DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),
			       COALESCE(t.name,''),
			       COALESCE(u.display_name,''),
			       c.team_id, c.owner_id
			FROM audit_control c JOIN audit a ON a.id = c.audit_id
			LEFT JOIN audit_team t ON t.id = c.team_id
			LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
			%s ORDER BY c.due_date %s LIMIT ? OFFSET ?`, overdueWhere, dueDir) // #nosec G201
		pageArgs := append(append(args, filterArgs...), limit, offset)
		items, err = r.scanControlItems(ctx, q, pageArgs)
	}

	if err != nil {
		return nil, err
	}
	return &domain.WorkQueuePage{Items: items, Total: total, Page: page, Limit: limit}, nil
}

func (r *dashboardRepo) scanControlItems(ctx context.Context, q string, args []any) ([]domain.DashboardControlItem, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []domain.DashboardControlItem{}
	for rows.Next() {
		var item domain.DashboardControlItem
		if err := rows.Scan(&item.ControlID, &item.AuditID, &item.AuditName, &item.ControlNumber, &item.Description, &item.Status, &item.DueDate, &item.Team, &item.ProcessOwner, &item.TeamID, &item.OwnerID); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (r *dashboardRepo) queryOverdueControls(ctx context.Context, baseWhere string, scopeArgs []any) ([]domain.DashboardControlItem, error) {
	q := fmt.Sprintf(`
		SELECT c.id, c.audit_id, a.name,
		       c.control_number,
		       c.description,
		       c.status,
		       DATE_FORMAT(c.due_date,'%%Y-%%m-%%d'),
		       COALESCE(t.name,''),
		       COALESCE(u.display_name,''),
		       c.team_id, c.owner_id
		FROM audit_control c JOIN audit a ON a.id = c.audit_id
		LEFT JOIN audit_team t ON t.id = c.team_id
		LEFT JOIN `+"`user`"+` u ON u.id = c.owner_id
		%s AND c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE'
		ORDER BY c.due_date ASC LIMIT 100`, baseWhere) // #nosec G201
	return r.scanControlItems(ctx, q, scopeArgs)
}

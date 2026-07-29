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

// resolveScope returns a WHERE fragment (starting with "AND"), any args to bind,
// and an error. Only sql.ErrNoRows and a NULL team/user column are mapped to
// " AND 1=0" (legitimate no-data cases); any other DB error is propagated so
// callers return 500 instead of a silent empty dashboard.
func (r *dashboardRepo) resolveScope(ctx context.Context, req domain.AuditDashboardRequest) (string, []any, error) {
	switch req.PrimaryRole() {
	case domain.RoleComplianceAdmin, domain.RoleComplianceTeam, domain.RoleManagement:
		return "", nil, nil
	case domain.RoleInternalTeam:
		var teamID sql.NullInt64
		err := r.db.QueryRowContext(ctx, "SELECT audit_team_id FROM `user` WHERE email = ?", req.UserEmail).Scan(&teamID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !teamID.Valid) {
			return " AND 1=0", nil, nil
		}
		if err != nil {
			return "", nil, fmt.Errorf("dashboard.resolveScope: lookup team for %q: %w", req.UserEmail, err)
		}
		return " AND c.team_id = ?", []any{teamID.Int64}, nil
	case domain.RoleExternalAuditor:
		var userID sql.NullInt64
		err := r.db.QueryRowContext(ctx, "SELECT id FROM `user` WHERE email = ?", req.UserEmail).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !userID.Valid) {
			return " AND 1=0", nil, nil
		}
		if err != nil {
			return "", nil, fmt.Errorf("dashboard.resolveScope: lookup user for %q: %w", req.UserEmail, err)
		}
		return " AND c.auditor_id = ?", []any{userID.Int64}, nil
	default:
		return " AND 1=0", nil, nil
	}
}

func (r *dashboardRepo) Get(ctx context.Context, req domain.AuditDashboardRequest) (*domain.DashboardData, error) {
	scope, args, err := r.resolveScope(ctx, req)
	if err != nil {
		return nil, err
	}
	baseWhere := "WHERE a.status = 'ACTIVE'" + scope

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
	actionItems, err := r.queryActionItems(ctx, req, baseWhere, args)
	if err != nil {
		return nil, err
	}
	dueSoonItems, err := r.queryDueSoonItems(ctx, req, baseWhere, args)
	if err != nil {
		return nil, err
	}
	totalActionItems, err := r.queryActionItemsCount(ctx, req, baseWhere, args)
	if err != nil {
		return nil, err
	}
	overdueControls, err := r.queryOverdueControls(ctx, baseWhere, args)
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

func (r *dashboardRepo) actionItemsStatusFilter(role string) (string, bool) {
	switch role {
	case domain.RoleInternalTeam:
		return "c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION','POPULATION_PENDING','POPULATION_NEED_CLARIFICATION')", true
	case domain.RoleComplianceAdmin, domain.RoleComplianceTeam:
		return "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')", true
	case domain.RoleExternalAuditor:
		return "c.status IN ('EVIDENCE_UNDER_VALIDATION','POPULATION_UNDER_VALIDATION','POPULATION_COMPLETE','AWAITING_SAMPLE')", true
	case domain.RoleManagement:
		return "", false
	default:
		return "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')", true
	}
}

func (r *dashboardRepo) queryActionItems(ctx context.Context, req domain.AuditDashboardRequest, baseWhere string, scopeArgs []any) ([]domain.DashboardControlItem, error) {
	statusFilter, ok := r.actionItemsStatusFilter(req.PrimaryRole())
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

func (r *dashboardRepo) queryDueSoonItems(ctx context.Context, req domain.AuditDashboardRequest, baseWhere string, scopeArgs []any) ([]domain.DashboardControlItem, error) {
	statusFilter, ok := r.actionItemsStatusFilter(req.PrimaryRole())
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
		%s AND %s AND c.due_date IS NOT NULL
		AND c.due_date BETWEEN CURDATE() AND DATE_ADD(CURDATE(), INTERVAL 7 DAY)
		ORDER BY c.due_date ASC, c.id ASC LIMIT 500`, baseWhere, statusFilter) // #nosec G201
	return r.scanControlItems(ctx, q, scopeArgs)
}

func (r *dashboardRepo) queryActionItemsCount(ctx context.Context, req domain.AuditDashboardRequest, baseWhere string, scopeArgs []any) (int, error) {
	var statusFilter string
	switch req.PrimaryRole() {
	case domain.RoleInternalTeam:
		statusFilter = "c.status IN ('EVIDENCE_PENDING','SUBMITTED_SAMPLE','EVIDENCE_NEED_CLARIFICATION','POPULATION_PENDING','POPULATION_NEED_CLARIFICATION')"
	case domain.RoleComplianceAdmin, domain.RoleComplianceTeam:
		statusFilter = "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')"
	case domain.RoleExternalAuditor:
		statusFilter = "c.status IN ('EVIDENCE_UNDER_VALIDATION','POPULATION_UNDER_VALIDATION','POPULATION_COMPLETE','AWAITING_SAMPLE')"
	case domain.RoleManagement:
		return 0, nil
	default:
		statusFilter = "c.status IN ('EVIDENCE_INTERNAL_REVIEW','POPULATION_INTERNAL_REVIEW')"
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

// GetWorkQueuePage returns a single paginated page of work-queue items.
func (r *dashboardRepo) GetWorkQueuePage(ctx context.Context, req domain.WorkQueueRequest) (*domain.WorkQueuePage, error) {
	// Resolve scope the same way as the dashboard.
	dashReq := domain.AuditDashboardRequest{Roles: req.Roles, UserEmail: req.UserEmail}
	scope, args, err := r.resolveScope(ctx, dashReq)
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

	// Build optional team/owner filter fragments on the FK columns of audit_control,
	// so count queries need no extra JOINs (c.team_id and c.owner_id are on the base table).
	teamSQL, teamArgs := buildIntInFilter("c.team_id", req.TeamIDs)
	ownerSQL, ownerArgs := buildIntInFilter("c.owner_id", req.OwnerIDs)
	filterSQL := teamSQL + ownerSQL
	filterArgs := append(teamArgs, ownerArgs...)

	var items []domain.DashboardControlItem
	var total int

	switch req.Tab {
	case domain.WorkQueueTabActionItems:
		statusFilter, ok := r.actionItemsStatusFilter(req.PrimaryRole())
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
			%s AND %s%s ORDER BY c.due_date ASC, c.id ASC LIMIT ? OFFSET ?`, baseWhere, statusFilter, filterSQL) // #nosec G201
		pageArgs := append(append(args, filterArgs...), limit, offset)
		items, err = r.scanControlItems(ctx, q, pageArgs)

	case domain.WorkQueueTabDueSoon:
		statusFilter, ok := r.actionItemsStatusFilter(req.PrimaryRole())
		if !ok {
			return &domain.WorkQueuePage{Items: []domain.DashboardControlItem{}, Total: 0, Page: page, Limit: limit}, nil
		}
		dueSoonWhere := fmt.Sprintf(`%s AND %s AND c.due_date IS NOT NULL AND c.due_date BETWEEN CURDATE() AND DATE_ADD(CURDATE(), INTERVAL 7 DAY)%s`, baseWhere, statusFilter, filterSQL) // #nosec G201
		cq := fmt.Sprintf(`SELECT COUNT(*) FROM audit_control c JOIN audit a ON a.id = c.audit_id %s`, dueSoonWhere)                                                                        // #nosec G201
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
			%s ORDER BY c.due_date ASC, c.id ASC LIMIT ? OFFSET ?`, dueSoonWhere) // #nosec G201
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
			%s ORDER BY c.due_date ASC LIMIT ? OFFSET ?`, overdueWhere) // #nosec G201
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

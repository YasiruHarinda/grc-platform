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

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// ControlRepository defines persistence operations for the audit_control table.
type ControlRepository interface {
	SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error)
	SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error)
	GetControlByID(ctx context.Context, auditID, controlID int) (*domain.AuditControl, error)
	CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (*domain.AuditControl, error)
	BulkCreateControls(ctx context.Context, auditID int, reqs []domain.CreateControlRequest) ([]domain.AuditControl, error)
	UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (*domain.AuditControl, error)
	// OverrideControlStatus writes a backward status override: the control's
	// status write, the demotion cascade of its dependent population/evidence
	// row (see overridePopulationTarget/overrideEvidenceTarget), and the derived
	// audit.status recompute all happen in one transaction, so a mid-flight
	// failure can never leave the control demoted but its dependent rows
	// stale. req.ExpectedStatus is the atomic concurrency guard, same as
	// UpdateControl.
	OverrideControlStatus(ctx context.Context, auditID, controlID int, req domain.OverrideControlStatusRequest) (*domain.AuditControl, error)
	DeleteControl(ctx context.Context, auditID, controlID int) error
	// CountDeletionBlockers returns how many audit_evidence rows exist for the
	// control and how many audit_population rows count as active: any status
	// other than PENDING (including the terminal APPROVED), plus any PENDING
	// round that already holds uploaded evidence files. Used to block
	// DeleteControl from silently cascading away real work (evidence/population
	// records cascade-delete with the control at the DB level).
	CountDeletionBlockers(ctx context.Context, controlID int) (evidenceCount int, activePopulationCount int, err error)
	// GetEvidenceAssignment returns the control's audit id when userID is the
	// control's owner and it is currently actionable, else sql.ErrNoRows.
	GetEvidenceAssignment(ctx context.Context, userID int, controlID int) (int, error)
	// FindActivePopulation returns the active audit_population id for an OE control
	// (status PENDING or COMPLIANCE_REJECTED), else sql.ErrNoRows.
	FindActivePopulation(ctx context.Context, controlID int) (int, error)
}

// evidenceActionableStatuses lists the control statuses for which the owner
// may still submit (population or evidence).
const evidenceActionableStatuses = `'POPULATION_PENDING','POPULATION_NEED_CLARIFICATION',
		'EVIDENCE_PENDING','EVIDENCE_NEED_CLARIFICATION','SUBMITTED_SAMPLE'`

type controlRepo struct{ db *sql.DB }

// NewControlRepository constructs a ControlRepository.
func NewControlRepository(db *sql.DB) ControlRepository { return &controlRepo{db: db} }

// GetEvidenceAssignment returns the control's audit id when the user is the
// control's owner and it is currently actionable. Returning the audit id lets the
// GRC Backend both (a) confirm assignment and (b) derive the audit for folder-path
// binding from the DB, so the client never supplies it. Not found → sql.ErrNoRows.
func (r *controlRepo) GetEvidenceAssignment(ctx context.Context, userID int, controlID int) (int, error) {
	var auditID int
	err := r.db.QueryRowContext(ctx, `
		SELECT c.audit_id
		FROM audit_control c
		JOIN audit  a ON a.id = c.audit_id
		WHERE c.owner_id = ? AND c.id = ?
		  AND a.status = 'ACTIVE'
		  AND c.status IN (`+evidenceActionableStatuses+`)
		LIMIT 1`, userID, controlID).Scan(&auditID)
	if err != nil {
		return 0, err
	}
	return auditID, nil
}

// FindActivePopulation returns the active population round for an OE control:
// PENDING (first submission), SUBMITTED (still under internal review — the team
// can keep adding files up until the reviewer decides, same as
// teamEditablePopulationStatuses on the backend), COMPLIANCE_REJECTED (internal
// review sent it back), or AUDITOR_REJECTED (auditor sent it back) — all four are
// states from which the population state machine allows a transition straight
// back to SUBMITTED on the same round (see allowedPopulationTransitions in
// audit_population_service.go; SUBMITTED -> SUBMITTED is the no-op case).
// Not found (no active population / DESIGN control) → sql.ErrNoRows.
func (r *controlRepo) FindActivePopulation(ctx context.Context, controlID int) (int, error) {
	var populationID int
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM audit_population
		WHERE control_id = ? AND status IN ('PENDING','SUBMITTED','COMPLIANCE_REJECTED','AUDITOR_REJECTED')
		ORDER BY id DESC LIMIT 1`, controlID).Scan(&populationID)
	if err != nil {
		return 0, err
	}
	return populationID, nil
}

const controlSelectCols = `
  c.id, c.audit_id,
  c.control_number, c.description, c.evidence_requirement,
  c.requirement_type, c.control_type, c.scope,
  c.owner_id,   u_owner.uuid AS owner_uuid, u_owner.user_type AS owner_user_type,
  c.team_id,    t.name       AS team_name,
  c.auditor_id, u_aud.uuid   AS auditor_uuid, u_aud.user_type AS auditor_user_type,
  DATE_FORMAT(c.due_date, '%Y-%m-%d') AS due_date,
  c.status, c.sample_reference, c.comments, c.control_source,
  (c.due_date IS NOT NULL AND c.due_date < CURDATE() AND c.status != 'COMPLETE') AS is_overdue,
  c.created_at, c.updated_at,
  c.status_overridden, c.overridden_by, c.overridden_at,
  p.description                        AS population_description,
  p.comments                           AS population_comments,
  DATE_FORMAT(p.due_date, '%Y-%m-%d')  AS population_due_date,
  u_pop_owner.uuid                     AS population_owner_uuid,
  u_pop_owner.user_type                AS population_owner_user_type,
  pop_team.name                        AS population_team_name,
  p.id                                 AS population_id,
  p.owner_id                           AS population_owner_id,
  p.status                             AS population_status`

const controlFromClause = `
FROM audit_control c
LEFT JOIN ` + "`user`" + ` u_owner ON u_owner.id = c.owner_id
LEFT JOIN audit_team t            ON t.id          = c.team_id
LEFT JOIN ` + "`user`" + ` u_aud   ON u_aud.id     = c.auditor_id
LEFT JOIN audit_population p      ON p.control_id  = c.id
    -- Highest id = current round, matching FindActivePopulation/
    -- demotePopulationRound/controlService.Update's own "most recent round"
    -- convention elsewhere in this module — not MIN(id), which would pin
    -- every consumer (control list/detail, the reminder sweep) to a
    -- control's very first population round forever, ignoring any
    -- resubmission cycle.
    AND p.id = (SELECT MAX(id) FROM audit_population WHERE control_id = c.id)
LEFT JOIN ` + "`user`" + ` u_pop_owner ON u_pop_owner.id = p.owner_id
LEFT JOIN audit_team pop_team         ON pop_team.id     = p.team_id`

func (r *controlRepo) SearchControls(ctx context.Context, auditID int, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error) {
	where, args := buildControlFilters("WHERE c.audit_id = ?", []any{auditID}, req)
	return r.runControlSearch(ctx, where, args, req, "control.Search")
}

func (r *controlRepo) SearchControlsGlobal(ctx context.Context, req domain.SearchControlsRequest) ([]domain.AuditControl, int, error) {
	where, args := buildControlFilters("WHERE 1=1", []any{}, req)
	return r.runControlSearch(ctx, where, args, req, "control.SearchGlobal")
}

// buildControlFilters appends the optional filter clauses from req onto seedWhere/seedArgs
// and returns the combined WHERE clause and argument list.
func buildControlFilters(seedWhere string, seedArgs []any, req domain.SearchControlsRequest) (string, []any) {
	where := seedWhere
	args := seedArgs

	if req.SearchQuery != "" {
		where += " AND (c.control_number LIKE ? OR c.description LIKE ?)"
		p := "%" + likeEscape(req.SearchQuery) + "%"
		args = append(args, p, p)
	}
	if len(req.StatusKeys) > 0 {
		ph := strings.Repeat("?,", len(req.StatusKeys))
		where += " AND c.status IN (" + ph[:len(ph)-1] + ")"
		for _, s := range req.StatusKeys {
			args = append(args, s)
		}
	}
	if len(req.RequirementTypes) > 0 {
		ph := strings.Repeat("?,", len(req.RequirementTypes))
		where += " AND c.requirement_type IN (" + ph[:len(ph)-1] + ")"
		for _, rt := range req.RequirementTypes {
			args = append(args, rt)
		}
	}
	if len(req.TeamIDs) > 0 {
		ph := strings.Repeat("?,", len(req.TeamIDs))
		where += " AND c.team_id IN (" + ph[:len(ph)-1] + ")"
		for _, id := range req.TeamIDs {
			args = append(args, id)
		}
	}
	if len(req.AuditorIDs) > 0 {
		ph := strings.Repeat("?,", len(req.AuditorIDs))
		where += " AND c.auditor_id IN (" + ph[:len(ph)-1] + ")"
		for _, id := range req.AuditorIDs {
			args = append(args, id)
		}
	}
	if len(req.OwnerIDs) > 0 {
		ph := strings.Repeat("?,", len(req.OwnerIDs))
		where += " AND c.owner_id IN (" + ph[:len(ph)-1] + ")"
		for _, id := range req.OwnerIDs {
			args = append(args, id)
		}
	}
	if len(req.ControlIDs) > 0 {
		ph := strings.Repeat("?,", len(req.ControlIDs))
		where += " AND c.id IN (" + ph[:len(ph)-1] + ")"
		for _, id := range req.ControlIDs {
			args = append(args, id)
		}
	}
	scopeClause, scopeArgs := controlScopeWhere(req.Scope, req.UserID, req.ScopeTeamIDs)
	where += scopeClause
	args = append(args, scopeArgs...)
	return where, args
}

// controlScopeWhere returns a WHERE fragment (starting with "AND") and its
// bind args for the given row scope, mirroring audit_dashboard_repo.go's
// scopeWhere (same `c` control alias, same owner_id/auditor_id match against
// the caller's own user id) so list/search endpoints enforce the same
// row-scoping rule the dashboard already does. A pure string/args builder,
// callable from buildControlFilters without a context or *sql.DB — the caller
// already has the resolved user id, so no DB round-trip is needed here.
func controlScopeWhere(scope domain.Scope, userID int, scopeTeamIDs []int) (string, []any) {
	switch scope {
	case domain.ScopeAll:
		return "", nil
	case domain.ScopeOwned:
		return " AND c.owner_id = ?", []any{userID}
	case domain.ScopeAssigned:
		return " AND c.auditor_id = ?", []any{userID}
	case domain.ScopeTeam:
		pred, args := teamScopePredicate("c", scopeTeamIDs, userID)
		return " AND " + pred, args
	default: // ScopeNone and any unrecognized value scope to nothing.
		return " AND 1=0", nil
	}
}

// teamScopePredicate builds the additive ScopeTeam predicate shared by every
// scopeWhere variant in this package: the caller's team's work OR anything
// they personally own OR anything they audit, keyed off
// alias.team_id/owner_id/auditor_id. A pure string/args builder — callers
// stay pure too, with no context or *sql.DB.
//
// Returns a bare parenthesized predicate with NO leading "AND" — callers
// combine it into their own clause (a plain "AND", or wrapped in an EXISTS for
// audits/frameworks, which have no team/owner/auditor of their own).
//
// Never a plain "alias.team_id IN (...)" on its own: that would take away a
// team lead's identity-based access to a record outside their own team. Never
// a bare "IN ()" when scopeTeamIDs is empty, and the caller must never receive
// an empty (no-filter) string in its place.
func teamScopePredicate(alias string, scopeTeamIDs []int, userID int) (string, []any) {
	terms := make([]string, 0, 3)
	args := make([]any, 0, len(scopeTeamIDs)+2)
	if len(scopeTeamIDs) > 0 {
		phs := strings.Repeat("?,", len(scopeTeamIDs))
		terms = append(terms, alias+".team_id IN ("+phs[:len(phs)-1]+")")
		for _, id := range scopeTeamIDs {
			args = append(args, id)
		}
	}
	terms = append(terms,
		alias+".owner_id   = ?",
		alias+".auditor_id = ?")
	args = append(args, userID, userID)
	return "(" + strings.Join(terms, " OR ") + ")", args
}

// runControlSearch executes the count + paginated data query and scans the results.
func (r *controlRepo) runControlSearch(ctx context.Context, where string, args []any, req domain.SearchControlsRequest, errPrefix string) ([]domain.AuditControl, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) "+controlFromClause+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s count: %w", errPrefix, err)
	}

	dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+" "+where+
			" ORDER BY c.control_number LIMIT ? OFFSET ?",
		dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s query: %w", errPrefix, err)
	}
	defer rows.Close()

	var controls []domain.AuditControl
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("%s scan: %w", errPrefix, err)
		}
		controls = append(controls, *c)
	}
	return controls, total, rows.Err()
}

func (r *controlRepo) GetControlByID(ctx context.Context, auditID, controlID int) (*domain.AuditControl, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+" WHERE c.audit_id = ? AND c.id = ?",
		auditID, controlID)
	c, err := scanControl(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("control %d not found in audit %d", controlID, auditID)}
	}
	if err != nil {
		return nil, fmt.Errorf("control.GetByID(%d,%d): %w", auditID, controlID, err)
	}
	return c, nil
}

func (r *controlRepo) CreateControl(ctx context.Context, auditID int, req domain.CreateControlRequest) (*domain.AuditControl, error) {
	controlSource := req.ControlSource
	if controlSource == "" {
		controlSource = "MANUAL"
	}
	initialStatus := "EVIDENCE_PENDING"
	if req.RequirementType == "OE" {
		initialStatus = "POPULATION_PENDING"
	}
	defCols := controlDefinitionCols(req)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.Create begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO audit_control
		 (audit_id,
		  control_number, description, evidence_requirement, requirement_type, control_type, scope,
		  owner_id, team_id, auditor_id, due_date, status, control_source, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		auditID,
		defCols.controlNumber, defCols.description, defCols.evidenceReq,
		defCols.requirementType, defCols.controlType, defCols.scope,
		nullableInt(req.OwnerID), nullableInt(req.TeamID), nullableInt(req.AuditorID),
		req.DueDate,
		initialStatus,
		controlSource,
		req.CreatedBy, req.CreatedBy)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, &apierror.ConflictError{Msg: fmt.Sprintf("control number %q already exists in this audit", defCols.controlNumber)}
		}
		return nil, fmt.Errorf("control.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	if req.Population != nil && strings.EqualFold(req.RequirementType, "OE") {
		p := req.Population
		desc := nullableString(&p.Description)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO audit_population
			 (control_id, owner_id, team_id, reference_number, description, due_date, comments, status, created_by, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', ?, ?)`,
			id, nullableInt(p.OwnerID), nullableInt(p.TeamID),
			p.ReferenceNumber, desc, p.DueDate, nullableString(p.Comments),
			req.CreatedBy, req.CreatedBy); err != nil {
			return nil, fmt.Errorf("control.Create population: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.Create commit: %w", err)
	}
	return r.GetControlByID(ctx, auditID, int(id))
}

func (r *controlRepo) BulkCreateControls(ctx context.Context, auditID int, reqs []domain.CreateControlRequest) ([]domain.AuditControl, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.BulkCreate begin: %w", err)
	}
	defer tx.Rollback()

	var ids []int
	for _, req := range reqs {
		controlSource := req.ControlSource
		if controlSource == "" {
			controlSource = "MANUAL"
		}
		initialStatus := "EVIDENCE_PENDING"
		if req.RequirementType == "OE" {
			initialStatus = "POPULATION_PENDING"
		}
		defCols := controlDefinitionCols(req)
		res, err := tx.ExecContext(ctx,
			`INSERT INTO audit_control
			 (audit_id,
			  control_number, description, evidence_requirement, requirement_type, control_type, scope,
			  owner_id, team_id, auditor_id, due_date, status, control_source, created_by, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			auditID,
			defCols.controlNumber, defCols.description, defCols.evidenceReq,
			defCols.requirementType, defCols.controlType, defCols.scope,
			nullableInt(req.OwnerID), nullableInt(req.TeamID), nullableInt(req.AuditorID),
			req.DueDate, initialStatus, controlSource,
			req.CreatedBy, req.CreatedBy)
		if err != nil {
			if isDuplicateKey(err) {
				return nil, &apierror.ConflictError{Msg: fmt.Sprintf("control number %q already exists in this audit", req.ControlNumber)}
			}
			return nil, fmt.Errorf("control.BulkCreate insert %q: %w", req.ControlNumber, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, int(id))
		if req.Population != nil && strings.EqualFold(req.RequirementType, "OE") {
			p := req.Population
			desc := nullableString(&p.Description)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO audit_population
				 (control_id, owner_id, team_id, reference_number, description, due_date, comments, status, created_by, updated_by)
				 VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', ?, ?)`,
				id, nullableInt(p.OwnerID), nullableInt(p.TeamID),
				p.ReferenceNumber, desc, p.DueDate, nullableString(p.Comments),
				req.CreatedBy, req.CreatedBy); err != nil {
				return nil, fmt.Errorf("control.BulkCreate population %q: %w", req.ControlNumber, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.BulkCreate commit: %w", err)
	}

	ph := strings.Repeat("?,", len(ids))
	inArgs := []any{auditID}
	for _, id := range ids {
		inArgs = append(inArgs, id)
	}
	rows, err := r.db.QueryContext(ctx,
		"SELECT"+controlSelectCols+controlFromClause+
			" WHERE c.audit_id = ? AND c.id IN ("+ph[:len(ph)-1]+")"+
			" ORDER BY c.control_number",
		inArgs...)
	if err != nil {
		return nil, fmt.Errorf("control.BulkCreate fetch: %w", err)
	}
	defer rows.Close()

	var controls []domain.AuditControl
	for rows.Next() {
		c, err := scanControl(rows)
		if err != nil {
			return nil, fmt.Errorf("control.BulkCreate fetch scan: %w", err)
		}
		controls = append(controls, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("control.BulkCreate fetch rows: %w", err)
	}
	return controls, nil
}

func (r *controlRepo) DeleteControl(ctx context.Context, auditID, controlID int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("control.Delete(%d,%d) begin: %w", auditID, controlID, err)
	}
	defer tx.Rollback()

	// Clear the send-log rows first: fk_notif_control / fk_notif_population are
	// ON DELETE RESTRICT, so the audit_control cascade would otherwise fail 1451.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM audit_notification
		 WHERE control_id = ?
		    OR population_id IN (SELECT id FROM audit_population WHERE control_id = ?)`,
		controlID, controlID); err != nil {
		return fmt.Errorf("control.Delete(%d,%d) notifications: %w", auditID, controlID, err)
	}

	result, err := tx.ExecContext(ctx,
		"DELETE FROM audit_control WHERE audit_id = ? AND id = ?", auditID, controlID)
	if err != nil {
		return fmt.Errorf("control.Delete(%d,%d): %w", auditID, controlID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return &apierror.NotFoundError{Msg: fmt.Sprintf("control %d not found in audit %d", controlID, auditID)}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("control.Delete(%d,%d) commit: %w", auditID, controlID, err)
	}
	return nil
}

func (r *controlRepo) CountDeletionBlockers(ctx context.Context, controlID int) (int, int, error) {
	var evidenceCount int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_evidence WHERE control_id = ?", controlID,
	).Scan(&evidenceCount); err != nil {
		return 0, 0, fmt.Errorf("control.CountDeletionBlockers evidence(%d): %w", controlID, err)
	}

	// PENDING is the freshly-created, never-submitted state every OE control's
	// population round starts in (see CreateControl/BulkCreateControls) — it
	// must not count as "in progress" or an OE control could never be deleted
	// before its team submits anything, unlike a DESIGN control (which has no
	// audit_population row at all until work starts). But uploads land before
	// submit flips the status away from PENDING, so a PENDING round can still
	// hold real files — those must block deletion too, same as an APPROVED
	// round (completed audit evidence), which is never safe to cascade away.
	var activePopulationCount int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_population p
		 WHERE p.control_id = ?
		   AND (p.status <> 'PENDING'
		        OR EXISTS (SELECT 1 FROM audit_evidence_file f WHERE f.population_id = p.id))`,
		controlID,
	).Scan(&activePopulationCount); err != nil {
		return 0, 0, fmt.Errorf("control.CountDeletionBlockers population(%d): %w", controlID, err)
	}

	return evidenceCount, activePopulationCount, nil
}

func (r *controlRepo) UpdateControl(ctx context.Context, auditID, controlID int, req domain.UpdateControlRequest) (*domain.AuditControl, error) {
	sets := []string{}
	args := []any{}

	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if req.ControlType != nil {
		sets = append(sets, "control_type = ?")
		args = append(args, *req.ControlType)
	}
	if req.Scope != nil {
		sets = append(sets, "scope = ?")
		args = append(args, *req.Scope)
	}
	if req.EvidenceRequirement != nil {
		sets = append(sets, "evidence_requirement = ?")
		args = append(args, *req.EvidenceRequirement)
	}
	if req.ClearOwner {
		sets = append(sets, "owner_id = ?")
		args = append(args, nil)
	} else if req.OwnerID != nil {
		sets = append(sets, "owner_id = ?")
		args = append(args, *req.OwnerID)
	}
	if req.ClearTeam {
		sets = append(sets, "team_id = ?")
		args = append(args, nil)
	} else if req.TeamID != nil {
		sets = append(sets, "team_id = ?")
		args = append(args, *req.TeamID)
	}
	if req.ClearAuditor {
		sets = append(sets, "auditor_id = ?")
		args = append(args, nil)
	} else if req.AuditorID != nil {
		sets = append(sets, "auditor_id = ?")
		args = append(args, *req.AuditorID)
	}
	if req.DueDate != nil {
		sets = append(sets, "due_date = ?")
		args = append(args, *req.DueDate)
	}
	if req.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *req.Status)
	}
	if req.Comments != nil {
		sets = append(sets, "comments = ?")
		args = append(args, *req.Comments)
	}
	if req.SampleReference != nil {
		sets = append(sets, "sample_reference = ?")
		args = append(args, *req.SampleReference)
	}
	sets = append(sets, "updated_by = ?")
	args = append(args, req.UpdatedBy)

	var query string
	if req.ExpectedStatus != "" {
		args = append(args, auditID, controlID, req.ExpectedStatus)
		query = "UPDATE audit_control SET " + strings.Join(sets, ", ") + " WHERE audit_id = ? AND id = ? AND status = ?" // #nosec G202
	} else {
		args = append(args, auditID, controlID)
		query = "UPDATE audit_control SET " + strings.Join(sets, ", ") + " WHERE audit_id = ? AND id = ?" // #nosec G202
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.Update begin: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("control.Update(%d,%d): %w", auditID, controlID, err)
	}
	if req.ExpectedStatus != "" {
		if n, _ := result.RowsAffected(); n == 0 {
			var currentStatus string
			err := tx.QueryRowContext(ctx, "SELECT status FROM audit_control WHERE audit_id = ? AND id = ?", auditID, controlID).Scan(&currentStatus)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("control %d not found in audit %d", controlID, auditID)}
			}
			if err != nil {
				return nil, fmt.Errorf("control.Update(%d,%d) recheck: %w", auditID, controlID, err)
			}
			if currentStatus != req.ExpectedStatus || (req.Status != nil && *req.Status != req.ExpectedStatus) {
				return nil, &apierror.ConflictError{Msg: "control was modified concurrently, please retry"}
			}
			// MySQL no-op: status not being changed, or being set to its current value.
		}
	}

	if req.Status != nil {
		if err := recomputeAuditStatus(ctx, tx, auditID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.Update commit: %w", err)
	}
	return r.GetControlByID(ctx, auditID, controlID)
}

// OverrideControlStatus writes the control's demoted status, cascades its
// dependent population/evidence row per overridePopulationTarget/
// overrideEvidenceTarget, and recomputes the derived audit status — all in one
// transaction, so a mid-flight failure can never strand the control demoted
// with a stale dependent row. The cascade itself exists to prevent two
// failure modes: a FindActivePopulation deadlock, and a stale round's status
// getting silently overwritten by a later reviewer decision.
func (r *controlRepo) OverrideControlStatus(ctx context.Context, auditID, controlID int, req domain.OverrideControlStatusRequest) (*domain.AuditControl, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("control.OverrideStatus begin: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`UPDATE audit_control
		 SET status = ?, status_overridden = TRUE, overridden_by = ?, overridden_at = NOW(), updated_by = ?
		 WHERE audit_id = ? AND id = ? AND status = ?`,
		req.Status, req.UpdatedBy, req.UpdatedBy, auditID, controlID, req.ExpectedStatus)
	if err != nil {
		return nil, fmt.Errorf("control.OverrideStatus(%d,%d): %w", auditID, controlID, err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil, &apierror.ConflictError{Msg: "control was modified concurrently, please retry"}
	}

	if popTarget, ok := overridePopulationTarget[req.Status]; ok {
		if err := demotePopulationRound(ctx, tx, controlID, popTarget); err != nil {
			return nil, err
		}
	}
	if evTarget, ok := overrideEvidenceTarget[req.Status]; ok {
		if err := demoteEvidenceRound(ctx, tx, controlID, evTarget); err != nil {
			return nil, err
		}
	}

	if err := recomputeAuditStatus(ctx, tx, auditID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("control.OverrideStatus commit: %w", err)
	}
	return r.GetControlByID(ctx, auditID, controlID)
}

// overridePopulationTarget maps a control override target to the
// audit_population status its dependent population row demotes to, when that
// row's current rank is strictly higher (see populationStatusRank). Targets
// not present here leave the population row untouched.
var overridePopulationTarget = map[string]string{
	"POPULATION_PENDING":            "PENDING",
	"POPULATION_INTERNAL_REVIEW":    "SUBMITTED",
	"POPULATION_UNDER_VALIDATION":   "COMPLIANCE_APPROVED",
	"POPULATION_NEED_CLARIFICATION": "AUDITOR_REJECTED",
	"POPULATION_COMPLETE":           "APPROVED",
	"AWAITING_SAMPLE":               "APPROVED",
	"SUBMITTED_SAMPLE":              "APPROVED",
}

// overrideEvidenceTarget is overridePopulationTarget's counterpart for the
// control's latest audit_evidence round.
var overrideEvidenceTarget = map[string]string{
	"EVIDENCE_INTERNAL_REVIEW":    "SUBMITTED",
	"EVIDENCE_UNDER_VALIDATION":   "COMPLIANCE_APPROVED",
	"EVIDENCE_NEED_CLARIFICATION": "AUDITOR_REJECTED",
}

// populationStatusRank orders audit_population.status for the cascade: a
// mapped overridePopulationTarget only applies when its rank is strictly
// below the row's current rank (never promotes a population row).
var populationStatusRank = map[string]int{
	"PENDING":             0,
	"COMPLIANCE_REJECTED": 0,
	"AUDITOR_REJECTED":    0,
	"SUBMITTED":           1,
	"COMPLIANCE_APPROVED": 2,
	"APPROVED":            3,
}

// evidenceStatusRank is populationStatusRank's counterpart for an
// audit_evidence round's status.
var evidenceStatusRank = map[string]int{
	"COMPLIANCE_REJECTED": 0,
	"AUDITOR_REJECTED":    0,
	"SUBMITTED":           1,
	"COMPLIANCE_APPROVED": 2,
	"APPROVED":            3,
}

// demotePopulationRound sets controlID's latest audit_population row to
// target, but only when target's rank is strictly below the row's current
// rank (populationStatusRank) — an override never promotes a dependent row.
// No-op if the control has no population row (DESIGN controls).
func demotePopulationRound(ctx context.Context, tx *sql.Tx, controlID int, target string) error {
	var id int
	var status string
	err := tx.QueryRowContext(ctx,
		`SELECT id, status FROM audit_population WHERE control_id = ? ORDER BY id DESC LIMIT 1`, controlID,
	).Scan(&id, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("control.OverrideStatus population lookup(%d): %w", controlID, err)
	}
	if populationStatusRank[target] >= populationStatusRank[status] {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE audit_population SET status = ? WHERE id = ?`, target, id); err != nil {
		return fmt.Errorf("control.OverrideStatus population update(%d): %w", id, err)
	}
	return nil
}

// demoteEvidenceRound is demotePopulationRound's counterpart for the
// control's latest audit_evidence round.
func demoteEvidenceRound(ctx context.Context, tx *sql.Tx, controlID int, target string) error {
	var id int
	var status string
	err := tx.QueryRowContext(ctx,
		`SELECT id, status FROM audit_evidence WHERE control_id = ? ORDER BY id DESC LIMIT 1`, controlID,
	).Scan(&id, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("control.OverrideStatus evidence lookup(%d): %w", controlID, err)
	}
	if evidenceStatusRank[target] >= evidenceStatusRank[status] {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE audit_evidence SET status = ? WHERE id = ?`, target, id); err != nil {
		return fmt.Errorf("control.OverrideStatus evidence update(%d): %w", id, err)
	}
	return nil
}

// recomputeAuditStatus derives audit.status from its controls' completion
// state: ACTIVE while any control is not COMPLETE, COMPLETED once every
// control is. ARCHIVED and REMOVED are admin-set and sticky — never
// overwritten here. Called after any control status write (ordinary or
// overridden) within the same transaction as that write.
func recomputeAuditStatus(ctx context.Context, tx *sql.Tx, auditID int) error {
	var current string
	if err := tx.QueryRowContext(ctx, "SELECT status FROM audit WHERE id = ? FOR UPDATE", auditID).Scan(&current); err != nil {
		return fmt.Errorf("recomputeAuditStatus get(%d): %w", auditID, err)
	}
	if current == "ARCHIVED" || current == "REMOVED" {
		return nil
	}
	var incomplete int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_control WHERE audit_id = ? AND status <> 'COMPLETE' FOR UPDATE", auditID,
	).Scan(&incomplete); err != nil {
		return fmt.Errorf("recomputeAuditStatus count(%d): %w", auditID, err)
	}
	target := "ACTIVE"
	if incomplete == 0 {
		target = "COMPLETED"
	}
	if target == current {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE audit SET status = ? WHERE id = ?", target, auditID); err != nil {
		return fmt.Errorf("recomputeAuditStatus update(%d): %w", auditID, err)
	}
	return nil
}

func scanControl(s scanner) (*domain.AuditControl, error) {
	var c domain.AuditControl
	var ownerID, teamID, auditorID sql.NullInt64
	var evidenceReq, ownerUUID, ownerUserType, teamName, auditorUUID, auditorUserType, dueDate sql.NullString
	var sampleReference, comments sql.NullString
	var overriddenBy sql.NullString
	var overriddenAt sql.NullTime
	var popDescription, popComments, popDueDate, popOwnerUUID, popOwnerUserType, popTeamName sql.NullString
	var popID, popOwnerID sql.NullInt64
	var popStatus sql.NullString
	err := s.Scan(
		&c.ID, &c.AuditID,
		&c.ControlNumber, &c.Description, &evidenceReq,
		&c.RequirementType, &c.ControlType, &c.Scope,
		&ownerID, &ownerUUID, &ownerUserType,
		&teamID, &teamName,
		&auditorID, &auditorUUID, &auditorUserType,
		&dueDate,
		&c.Status, &sampleReference, &comments, &c.ControlSource, &c.IsOverdue,
		&c.CreatedOn, &c.UpdatedOn,
		&c.StatusOverridden, &overriddenBy, &overriddenAt,
		&popDescription, &popComments, &popDueDate, &popOwnerUUID, &popOwnerUserType, &popTeamName,
		&popID, &popOwnerID, &popStatus,
	)
	if err != nil {
		return nil, err
	}
	nullStrPtr := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}
	nullIntPtr := func(ni sql.NullInt64) *int {
		if ni.Valid {
			v := int(ni.Int64)
			return &v
		}
		return nil
	}
	c.EvidenceRequirement = nullStrPtr(evidenceReq)
	c.OwnerID = nullIntPtr(ownerID)
	c.OwnerUUID = nullStrPtr(ownerUUID)
	c.OwnerUserType = nullStrPtr(ownerUserType)
	c.TeamID = nullIntPtr(teamID)
	c.TeamName = nullStrPtr(teamName)
	c.AuditorID = nullIntPtr(auditorID)
	c.AuditorUUID = nullStrPtr(auditorUUID)
	c.AuditorUserType = nullStrPtr(auditorUserType)
	c.DueDate = nullStrPtr(dueDate)
	c.SampleReference = nullStrPtr(sampleReference)
	c.Comments = nullStrPtr(comments)
	c.PopulationDescription = nullStrPtr(popDescription)
	c.PopulationComments = nullStrPtr(popComments)
	c.PopulationDueDate = nullStrPtr(popDueDate)
	c.PopulationOwnerUUID = nullStrPtr(popOwnerUUID)
	c.PopulationOwnerUserType = nullStrPtr(popOwnerUserType)
	c.PopulationTeamName = nullStrPtr(popTeamName)
	c.OverriddenBy = nullStrPtr(overriddenBy)
	if overriddenAt.Valid {
		c.OverriddenAt = &overriddenAt.Time
	}
	c.PopulationID = nullIntPtr(popID)
	c.PopulationOwnerID = nullIntPtr(popOwnerID)
	c.PopulationStatus = nullStrPtr(popStatus)
	return &c, nil
}

// controlDefCols holds the definition values to store in audit_control.
// Every control owns its full definition text; only evidenceReq is optional.
type controlDefCols struct {
	controlNumber, description, requirementType, controlType, scope string
	evidenceReq                                                     sql.NullString
}

// controlDefinitionCols returns the definition column values for an INSERT.
func controlDefinitionCols(req domain.CreateControlRequest) controlDefCols {
	d := controlDefCols{
		controlNumber:   req.ControlNumber,
		description:     req.Description,
		requirementType: req.RequirementType,
		controlType:     req.ControlType,
		scope:           req.Scope,
	}
	if req.EvidenceRequirement != nil {
		d.evidenceReq = sql.NullString{String: *req.EvidenceRequirement, Valid: true}
	}
	return d
}

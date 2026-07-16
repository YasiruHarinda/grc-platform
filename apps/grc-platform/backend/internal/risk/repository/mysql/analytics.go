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

package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

type analyticsRepository struct{ db *sql.DB }

// NewAnalyticsRepository creates a MySQL-backed repository.AnalyticsRepository.
func NewAnalyticsRepository(db *sql.DB) repository.AnalyticsRepository {
	return &analyticsRepository{db: db}
}

// registerFilter returns the SQL fragment and args for an optional
// r.source_register_id = ? clause, to be appended after an existing WHERE.
func registerFilter(registerID *int) (string, []any) {
	if registerID == nil {
		return "", nil
	}
	return " AND r.source_register_id = ?", []any{*registerID}
}

func (a *analyticsRepository) NewThisMonthCount(ctx context.Context, registerID *int, monthStart string) (int, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{monthStart}, filterArgs...)

	var count int
	err := a.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM risk r
		WHERE r.workflow_status <> 'CANCELLED'
		  AND r.risk_identified_date >= ?`+clause,
		args...,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("analytics new this month count: %w", err)
	}
	return count, nil
}

func (a *analyticsRepository) AvgDaysToClose(ctx context.Context, registerID *int) (*float64, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusClosed}, filterArgs...)

	var avg sql.NullFloat64
	err := a.db.QueryRowContext(ctx, `
		SELECT AVG(DATEDIFF(r.updated_at, r.risk_identified_date))
		FROM risk r
		WHERE r.workflow_status = ?
		  AND r.risk_identified_date IS NOT NULL`+clause,
		args...,
	).Scan(&avg)
	if err != nil {
		return nil, fmt.Errorf("analytics avg days to close: %w", err)
	}
	if !avg.Valid {
		return nil, nil
	}
	return &avg.Float64, nil
}

func (a *analyticsRepository) AvgEffectiveScore(ctx context.Context, registerID *int) (*float64, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusClosed, model.StatusCancelled}, filterArgs...)

	var avg sql.NullFloat64
	err := a.db.QueryRowContext(ctx, `
		SELECT AVG(rs.risk_rating)
		FROM risk r`+effectiveScoreJoin+`
		WHERE r.workflow_status NOT IN (?, ?)`+clause,
		args...,
	).Scan(&avg)
	if err != nil {
		return nil, fmt.Errorf("analytics avg effective score: %w", err)
	}
	if !avg.Valid {
		return nil, nil
	}
	return &avg.Float64, nil
}

func (a *analyticsRepository) IdentifiedTrend(ctx context.Context, registerID *int, since string) ([]model.MonthScoreStat, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusCancelled, since}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT DATE_FORMAT(r.risk_identified_date, '%Y-%m-01'), COUNT(*), AVG(rs.risk_rating)
		FROM risk r`+effectiveScoreJoin+`
		WHERE r.workflow_status <> ?
		  AND r.risk_identified_date >= ?`+clause+`
		GROUP BY 1
		ORDER BY 1`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics identified trend: %w", err)
	}
	defer rows.Close()

	var out []model.MonthScoreStat
	for rows.Next() {
		var s model.MonthScoreStat
		if err := rows.Scan(&s.Month, &s.Count, &s.AvgScore); err != nil {
			return nil, fmt.Errorf("scan identified trend row: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) ClosedTrend(ctx context.Context, registerID *int, since string) ([]model.MonthCount, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusClosed, since}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT DATE_FORMAT(r.updated_at, '%Y-%m-01'), COUNT(*)
		FROM risk r
		WHERE r.workflow_status = ?
		  AND r.updated_at >= ?`+clause+`
		GROUP BY 1
		ORDER BY 1`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics closed trend: %w", err)
	}
	defer rows.Close()

	var out []model.MonthCount
	for rows.Next() {
		var c model.MonthCount
		if err := rows.Scan(&c.Month, &c.Count); err != nil {
			return nil, fmt.Errorf("scan closed trend row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) LevelDistribution(ctx context.Context, registerID *int, since string) ([]model.MonthLevelCount, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusCancelled, since}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT DATE_FORMAT(r.risk_identified_date, '%Y-%m-01'), rs.risk_level, rs.color_code, COUNT(*)
		FROM risk r`+effectiveScoreJoin+`
		WHERE r.workflow_status <> ?
		  AND r.risk_identified_date >= ?`+clause+`
		GROUP BY 1, rs.risk_level, rs.color_code
		ORDER BY 1`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics level distribution: %w", err)
	}
	defer rows.Close()

	var out []model.MonthLevelCount
	for rows.Next() {
		var m model.MonthLevelCount
		if err := rows.Scan(&m.Month, &m.RiskLevel, &m.ColorCode, &m.Count); err != nil {
			return nil, fmt.Errorf("scan level distribution row: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) LevelReference(ctx context.Context) ([]model.RiskLevelRef, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT risk_level, MIN(color_code)
		FROM risk_score
		GROUP BY risk_level
		ORDER BY MAX(risk_rating) DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics level reference: %w", err)
	}
	defer rows.Close()

	var out []model.RiskLevelRef
	for rows.Next() {
		var l model.RiskLevelRef
		if err := rows.Scan(&l.RiskLevel, &l.ColorCode); err != nil {
			return nil, fmt.Errorf("scan level reference row: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) IdentifiedTrendByRegister(ctx context.Context, registerID *int, since string) ([]model.MonthRegisterCount, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusCancelled, since}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT DATE_FORMAT(r.risk_identified_date, '%Y-%m-01'), st.name, COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id
		WHERE r.workflow_status <> ?
		  AND r.risk_identified_date >= ?`+clause+`
		GROUP BY 1, st.name
		ORDER BY 1`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics identified trend by register: %w", err)
	}
	defer rows.Close()

	var out []model.MonthRegisterCount
	for rows.Next() {
		var m model.MonthRegisterCount
		if err := rows.Scan(&m.Month, &m.RegisterName, &m.Count); err != nil {
			return nil, fmt.Errorf("scan identified trend by register row: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) ClosedTrendByRegister(ctx context.Context, registerID *int, since string) ([]model.MonthRegisterCount, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusClosed, since}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT DATE_FORMAT(r.updated_at, '%Y-%m-01'), st.name, COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id
		WHERE r.workflow_status = ?
		  AND r.updated_at >= ?`+clause+`
		GROUP BY 1, st.name
		ORDER BY 1`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics closed trend by register: %w", err)
	}
	defer rows.Close()

	var out []model.MonthRegisterCount
	for rows.Next() {
		var m model.MonthRegisterCount
		if err := rows.Scan(&m.Month, &m.RegisterName, &m.Count); err != nil {
			return nil, fmt.Errorf("scan closed trend by register row: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) RegisterTotals(ctx context.Context) ([]model.RegisterShare, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT st.name, COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id
		WHERE r.workflow_status <> ?
		GROUP BY st.name
		ORDER BY COUNT(*) DESC`,
		model.StatusCancelled,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics register totals: %w", err)
	}
	defer rows.Close()

	var out []model.RegisterShare
	for rows.Next() {
		var s model.RegisterShare
		if err := rows.Scan(&s.RegisterName, &s.Count); err != nil {
			return nil, fmt.Errorf("scan register total row: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) ComplianceDistribution(ctx context.Context, registerID *int) ([]model.ComplianceShare, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusCancelled}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT ref.name, COUNT(*)
		FROM risk r
		JOIN risk_compliance_reference rc ON rc.risk_id = r.id
		JOIN risk_security_compliance_reference ref ON ref.id = rc.reference_id
		WHERE r.workflow_status <> ?`+clause+`
		GROUP BY ref.name
		ORDER BY COUNT(*) DESC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics compliance distribution: %w", err)
	}
	defer rows.Close()

	var out []model.ComplianceShare
	for rows.Next() {
		var c model.ComplianceShare
		if err := rows.Scan(&c.ComplianceName, &c.Count); err != nil {
			return nil, fmt.Errorf("scan compliance distribution row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) TreatmentMix(ctx context.Context, registerID *int) ([]model.TreatmentShare, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusClosed, model.StatusCancelled}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT COALESCE(r.treatment_strategy, ''), COUNT(*)
		FROM risk r
		WHERE r.workflow_status NOT IN (?, ?)`+clause+`
		GROUP BY r.treatment_strategy
		ORDER BY COUNT(*) DESC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics treatment mix: %w", err)
	}
	defer rows.Close()

	var out []model.TreatmentShare
	for rows.Next() {
		var t model.TreatmentShare
		if err := rows.Scan(&t.TreatmentStrategy, &t.Count); err != nil {
			return nil, fmt.Errorf("scan treatment mix row: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) WorkflowFunnel(ctx context.Context, registerID *int) ([]model.WorkflowStageCount, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusCancelled}, filterArgs...)

	rows, err := a.db.QueryContext(ctx, `
		SELECT r.workflow_status, COUNT(*)
		FROM risk r
		WHERE r.workflow_status <> ?`+clause+`
		GROUP BY r.workflow_status`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics workflow funnel: %w", err)
	}
	defer rows.Close()

	var out []model.WorkflowStageCount
	for rows.Next() {
		var w model.WorkflowStageCount
		if err := rows.Scan(&w.WorkflowStatus, &w.Count); err != nil {
			return nil, fmt.Errorf("scan workflow funnel row: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (a *analyticsRepository) AgingRisks(ctx context.Context, registerID *int, limit int) ([]model.AgingRiskItem, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{model.StatusClosed, model.StatusCancelled}, filterArgs...)
	args = append(args, limit)

	rows, err := a.db.QueryContext(ctx, `
		SELECT r.id, r.risk_code, r.risk_title, st.name,
		       COALESCE(owner.display_name, ''), rs.risk_level, rs.color_code,
		       r.risk_identified_date, DATEDIFF(CURDATE(), r.risk_identified_date)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+effectiveScoreJoin+`
		LEFT JOIN `+"`user`"+` owner ON owner.id = r.owner_id
		WHERE r.workflow_status NOT IN (?, ?)
		  AND r.risk_identified_date IS NOT NULL`+clause+`
		ORDER BY r.risk_identified_date ASC, r.id ASC
		LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("analytics aging risks: %w", err)
	}
	defer rows.Close()

	var out []model.AgingRiskItem
	for rows.Next() {
		var it model.AgingRiskItem
		if err := rows.Scan(
			&it.ID, &it.RiskCode, &it.RiskTitle, &it.RegisterName,
			&it.OwnerName, &it.RiskLevel, &it.ColorCode,
			&it.IdentifiedDate, &it.AgeDays,
		); err != nil {
			return nil, fmt.Errorf("scan aging risk row: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

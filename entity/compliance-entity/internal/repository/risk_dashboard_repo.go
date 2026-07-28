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
	"fmt"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// Risk workflow statuses this file filters on. Declared here rather than
// imported so the SQL below reads as SQL.
const (
	statusClosed    = "CLOSED"
	statusCancelled = "CANCELLED"
)

// RiskDashboardRepository provides the aggregated reads behind the risk
// dashboard. Every method excludes CANCELLED risks; "open" means any status
// other than CLOSED. registerID nil means every register.
type RiskDashboardRepository interface {
	StatusCounts(ctx context.Context, registerID *int) (*domain.RiskStatusSummary, error)
	OpenRiskFacts(ctx context.Context, registerID *int) ([]domain.OpenRiskFact, error)
	RegisterStatusFacts(ctx context.Context, registerID *int) ([]domain.RegisterStatusFact, error)
	CertTagCounts(ctx context.Context, registerID *int) ([]domain.RegisterCertCount, error)
	RepeatedComplianceRisks(ctx context.Context, registerID *int) ([]domain.RepeatedRiskRow, error)
	HighRisks(ctx context.Context, registerID *int) ([]domain.HighRiskItem, error)
	LevelOrder(ctx context.Context) ([]string, error)
}

type riskDashboardRepo struct{ db *sql.DB }

// NewRiskDashboardRepository constructs a RiskDashboardRepository.
func NewRiskDashboardRepository(db *sql.DB) RiskDashboardRepository {
	return &riskDashboardRepo{db: db}
}

// dashboardScoreJoin resolves each risk's effective residual score: the score
// of its latest reassessment when one exists, else its gross score. This is an
// inner join — a risk with neither drops out of score-based charts, which is
// acceptable for an aggregate but not for a list, where the risk read path uses
// a LEFT JOIN instead.
const dashboardScoreJoin = `
	JOIN risk_score rs ON rs.id = COALESCE(
		(SELECT ra.score_id
		   FROM risk_assessment ra
		  WHERE ra.risk_id = r.id
		  ORDER BY ra.created_at DESC, ra.id DESC
		  LIMIT 1),
		r.gross_score_id)`

// registerFilter returns an optional " AND r.source_register_id = ?" clause and
// its argument, so every query scopes the same way.
func registerFilter(registerID *int) (string, []any) {
	if registerID == nil {
		return "", nil
	}
	return " AND r.source_register_id = ?", []any{*registerID}
}

func (d *riskDashboardRepo) StatusCounts(ctx context.Context, registerID *int) (*domain.RiskStatusSummary, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{statusClosed, statusClosed, statusClosed, statusCancelled}, filterArgs...)

	var s domain.RiskStatusSummary
	err := d.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN r.workflow_status <> ? THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN r.workflow_status =  ? THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN r.workflow_status <> ?
		                           AND r.implementation_date IS NOT NULL
		                           AND r.implementation_date < CURDATE() THEN 1 ELSE 0 END), 0)
		FROM risk r
		WHERE r.workflow_status <> ?`+clause,
		args...,
	).Scan(&s.Total, &s.Open, &s.Closed, &s.Overdue)
	if err != nil {
		return nil, fmt.Errorf("risk dashboard status counts: %w", err)
	}
	return &s, nil
}

func (d *riskDashboardRepo) OpenRiskFacts(ctx context.Context, registerID *int) ([]domain.OpenRiskFact, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{statusClosed, statusCancelled}, filterArgs...)

	rows, err := d.db.QueryContext(ctx, `
		SELECT st.id, st.name, rs.likelihood, rs.impact, rs.risk_level, rs.color_code,
		       COALESCE(r.treatment_strategy, 'UNSPECIFIED'), COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+dashboardScoreJoin+`
		WHERE r.workflow_status NOT IN (?, ?)`+clause+`
		GROUP BY st.id, st.name, rs.likelihood, rs.impact, rs.risk_level, rs.color_code,
		         r.treatment_strategy
		ORDER BY st.name, rs.likelihood, rs.impact`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("risk dashboard open risk facts: %w", err)
	}
	defer rows.Close()

	var out []domain.OpenRiskFact
	for rows.Next() {
		var f domain.OpenRiskFact
		if err := rows.Scan(
			&f.RegisterID, &f.RegisterName, &f.Likelihood, &f.Impact,
			&f.RiskLevel, &f.ColorCode, &f.TreatmentStrategy, &f.Count,
		); err != nil {
			return nil, fmt.Errorf("scan open risk fact: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// registerStatusBucketCase is shared verbatim between RegisterStatusFacts'
// SELECT and GROUP BY clauses. It must be the exact same text in both places
// (not two separately-parameterised copies): under sql_mode=ONLY_FULL_GROUP_BY,
// MySQL treats each `?` placeholder as a distinct, value-unknown parameter, so
// two textually-identical CASE expressions built from separate placeholders are
// not recognised as the same expression and the query is rejected. The status
// constants are trusted Go literals, not user input, so inlining them is safe.
const registerStatusBucketCase = `CASE WHEN r.workflow_status = '` + statusClosed + `' THEN 'CLOSED'
	                ELSE COALESCE(r.treatment_strategy, 'UNSPECIFIED') END`

func (d *riskDashboardRepo) RegisterStatusFacts(ctx context.Context, registerID *int) ([]domain.RegisterStatusFact, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{statusCancelled}, filterArgs...)

	rows, err := d.db.QueryContext(ctx, `
		SELECT st.id, st.name, rs.risk_level, rs.color_code,
		       `+registerStatusBucketCase+`,
		       COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+dashboardScoreJoin+`
		WHERE r.workflow_status <> ?`+clause+`
		GROUP BY st.id, st.name, rs.risk_level, rs.color_code, `+registerStatusBucketCase+`
		ORDER BY st.name`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("risk dashboard register status facts: %w", err)
	}
	defer rows.Close()

	var out []domain.RegisterStatusFact
	for rows.Next() {
		var f domain.RegisterStatusFact
		if err := rows.Scan(
			&f.RegisterID, &f.RegisterName, &f.RiskLevel, &f.ColorCode, &f.Bucket, &f.Count,
		); err != nil {
			return nil, fmt.Errorf("scan register status fact: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (d *riskDashboardRepo) CertTagCounts(ctx context.Context, registerID *int) ([]domain.RegisterCertCount, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{statusClosed, statusCancelled}, filterArgs...)

	rows, err := d.db.QueryContext(ctx, `
		SELECT st.name, ref.name, COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id
		JOIN risk_compliance_reference rc ON rc.risk_id = r.id
		JOIN risk_security_compliance_reference ref ON ref.id = rc.reference_id
		WHERE r.workflow_status NOT IN (?, ?)`+clause+`
		GROUP BY st.name, ref.name
		ORDER BY st.name, ref.name`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("risk dashboard cert tag counts: %w", err)
	}
	defer rows.Close()

	var out []domain.RegisterCertCount
	for rows.Next() {
		var c domain.RegisterCertCount
		if err := rows.Scan(&c.RegisterName, &c.CertName, &c.Count); err != nil {
			return nil, fmt.Errorf("scan cert tag count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *riskDashboardRepo) RepeatedComplianceRisks(ctx context.Context, registerID *int) ([]domain.RepeatedRiskRow, error) {
	clause, filterArgs := registerFilter(registerID)
	r2Clause := ""
	if registerID != nil {
		r2Clause = " AND r2.source_register_id = ?"
	}
	args := []any{statusClosed, statusCancelled}
	args = append(args, filterArgs...)
	args = append(args, statusCancelled)
	args = append(args, filterArgs...)

	rows, err := d.db.QueryContext(ctx, `
		SELECT r.risk_title, st.name,
		       CASE WHEN r.workflow_status = ? THEN 'CLOSED' ELSE 'OPEN' END,
		       rs.risk_level, rs.color_code
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+dashboardScoreJoin+`
		WHERE r.workflow_status <> ?`+clause+`
		  AND EXISTS (SELECT 1 FROM risk_compliance_reference rc WHERE rc.risk_id = r.id)
		  AND r.risk_title IN (
		      SELECT r2.risk_title
		      FROM risk r2
		      WHERE r2.workflow_status <> ?`+r2Clause+`
		        AND EXISTS (SELECT 1 FROM risk_compliance_reference rc2 WHERE rc2.risk_id = r2.id)
		      GROUP BY r2.risk_title
		      HAVING COUNT(DISTINCT r2.source_register_id) >= 2)
		ORDER BY r.risk_title, st.name`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("risk dashboard repeated compliance risks: %w", err)
	}
	defer rows.Close()

	var out []domain.RepeatedRiskRow
	for rows.Next() {
		var r domain.RepeatedRiskRow
		if err := rows.Scan(&r.RiskTitle, &r.RegisterName, &r.Status, &r.RiskLevel, &r.ColorCode); err != nil {
			return nil, fmt.Errorf("scan repeated risk row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *riskDashboardRepo) HighRisks(ctx context.Context, registerID *int) ([]domain.HighRiskItem, error) {
	clause, filterArgs := registerFilter(registerID)
	args := append([]any{statusClosed, statusCancelled}, filterArgs...)

	rows, err := d.db.QueryContext(ctx, `
		SELECT r.id, r.risk_code, r.risk_title, st.name,
		       COALESCE(owner.display_name, ''),
		       DATE_FORMAT(r.risk_identified_date, '%Y-%m-%d'),
		       r.treatment_strategy,
		       DATE_FORMAT(r.implementation_date, '%Y-%m-%d')
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+dashboardScoreJoin+`
		LEFT JOIN `+"`user`"+` owner ON owner.id = r.owner_id
		WHERE r.workflow_status NOT IN (?, ?)`+clause+`
		  AND rs.risk_level = 'HIGH'
		ORDER BY r.risk_identified_date IS NULL, r.risk_identified_date ASC, r.id ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("risk dashboard high risks: %w", err)
	}
	defer rows.Close()

	var out []domain.HighRiskItem
	for rows.Next() {
		var h domain.HighRiskItem
		if err := rows.Scan(
			&h.ID, &h.RiskCode, &h.RiskTitle, &h.RegisterName, &h.OwnerName,
			&h.IdentifiedDate, &h.TreatmentStrategy, &h.ImplementationDate,
		); err != nil {
			return nil, fmt.Errorf("scan high risk item: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (d *riskDashboardRepo) LevelOrder(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT risk_level
		FROM risk_score
		GROUP BY risk_level
		ORDER BY MAX(risk_rating) DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("risk dashboard level order: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var level string
		if err := rows.Scan(&level); err != nil {
			return nil, fmt.Errorf("scan level order row: %w", err)
		}
		out = append(out, level)
	}
	return out, rows.Err()
}

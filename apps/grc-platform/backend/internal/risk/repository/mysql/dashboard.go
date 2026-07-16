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

type dashboardRepository struct{ db *sql.DB }

// NewDashboardRepository creates a MySQL-backed repository.DashboardRepository.
func NewDashboardRepository(db *sql.DB) repository.DashboardRepository {
	return &dashboardRepository{db: db}
}

// effectiveScoreJoin resolves each risk's effective residual score: the score
// of its latest reassessment when one exists, else its gross score. Risks with
// neither (no gross score and never assessed) drop out of score-based charts.
const effectiveScoreJoin = `
	JOIN risk_score rs ON rs.id = COALESCE(
		(SELECT ra.score_id
		   FROM risk_assessment ra
		  WHERE ra.risk_id = r.id
		  ORDER BY ra.created_at DESC, ra.id DESC
		  LIMIT 1),
		r.gross_score_id)`

func (d *dashboardRepository) StatusCounts(ctx context.Context) (*model.RiskStatusSummary, error) {
	var s model.RiskStatusSummary
	err := d.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN workflow_status <> ? THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN workflow_status =  ? THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN workflow_status <> ?
		                           AND implementation_date IS NOT NULL
		                           AND implementation_date < CURDATE() THEN 1 ELSE 0 END), 0)
		FROM risk
		WHERE workflow_status <> ?`,
		model.StatusClosed, model.StatusClosed, model.StatusClosed, model.StatusCancelled,
	).Scan(&s.Total, &s.Open, &s.Closed, &s.Overdue)
	if err != nil {
		return nil, fmt.Errorf("dashboard status counts: %w", err)
	}
	return &s, nil
}

func (d *dashboardRepository) OpenRiskFacts(ctx context.Context) ([]model.OpenRiskFact, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT st.id, st.name, rs.likelihood, rs.impact, rs.risk_level, rs.color_code,
		       COALESCE(r.treatment_strategy, 'UNSPECIFIED'), COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+effectiveScoreJoin+`
		WHERE r.workflow_status NOT IN (?, ?)
		GROUP BY st.id, st.name, rs.likelihood, rs.impact, rs.risk_level, rs.color_code,
		         r.treatment_strategy
		ORDER BY st.name, rs.likelihood, rs.impact`,
		model.StatusClosed, model.StatusCancelled,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard open risk facts: %w", err)
	}
	defer rows.Close()

	var out []model.OpenRiskFact
	for rows.Next() {
		var f model.OpenRiskFact
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

func (d *dashboardRepository) CertTagCounts(ctx context.Context) ([]model.RegisterCertCount, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT st.name, ref.name, COUNT(*)
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id
		JOIN risk_compliance_reference rc ON rc.risk_id = r.id
		JOIN risk_security_compliance_reference ref ON ref.id = rc.reference_id
		WHERE r.workflow_status NOT IN (?, ?)
		GROUP BY st.name, ref.name
		ORDER BY st.name, ref.name`,
		model.StatusClosed, model.StatusCancelled,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard cert tag counts: %w", err)
	}
	defer rows.Close()

	var out []model.RegisterCertCount
	for rows.Next() {
		var c model.RegisterCertCount
		if err := rows.Scan(&c.RegisterName, &c.CertName, &c.Count); err != nil {
			return nil, fmt.Errorf("scan cert tag count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *dashboardRepository) RepeatedComplianceRisks(ctx context.Context) ([]model.RepeatedRiskRow, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT r.risk_title, st.name,
		       CASE WHEN r.workflow_status = ? THEN 'CLOSED' ELSE 'OPEN' END,
		       rs.risk_level, rs.color_code
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+effectiveScoreJoin+`
		WHERE r.workflow_status <> ?
		  AND EXISTS (SELECT 1 FROM risk_compliance_reference rc WHERE rc.risk_id = r.id)
		  AND r.risk_title IN (
		      SELECT r2.risk_title
		      FROM risk r2
		      WHERE r2.workflow_status <> ?
		        AND EXISTS (SELECT 1 FROM risk_compliance_reference rc2 WHERE rc2.risk_id = r2.id)
		      GROUP BY r2.risk_title
		      HAVING COUNT(DISTINCT r2.source_register_id) >= 2)
		ORDER BY r.risk_title, st.name`,
		model.StatusClosed, model.StatusCancelled, model.StatusCancelled,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard repeated compliance risks: %w", err)
	}
	defer rows.Close()

	var out []model.RepeatedRiskRow
	for rows.Next() {
		var r model.RepeatedRiskRow
		if err := rows.Scan(&r.RiskTitle, &r.RegisterName, &r.Status, &r.RiskLevel, &r.ColorCode); err != nil {
			return nil, fmt.Errorf("scan repeated risk row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *dashboardRepository) HighRisks(ctx context.Context) ([]model.HighRiskItem, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT r.id, r.risk_code, r.risk_description, st.name,
		       COALESCE(owner.display_name, ''),
		       r.risk_identified_date, r.treatment_strategy, r.implementation_date
		FROM risk r
		JOIN risk_team st ON st.id = r.source_register_id`+effectiveScoreJoin+`
		LEFT JOIN `+"`user`"+` owner ON owner.id = r.owner_id
		WHERE r.workflow_status NOT IN (?, ?)
		  AND rs.risk_level = 'HIGH'
		ORDER BY r.risk_identified_date IS NULL, r.risk_identified_date ASC, r.id ASC`,
		model.StatusClosed, model.StatusCancelled,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard high risks: %w", err)
	}
	defer rows.Close()

	var out []model.HighRiskItem
	for rows.Next() {
		var h model.HighRiskItem
		if err := rows.Scan(
			&h.ID, &h.RiskCode, &h.RiskDescription, &h.RegisterName, &h.OwnerName,
			&h.IdentifiedDate, &h.TreatmentStrategy, &h.ImplementationDate,
		); err != nil {
			return nil, fmt.Errorf("scan high risk item: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

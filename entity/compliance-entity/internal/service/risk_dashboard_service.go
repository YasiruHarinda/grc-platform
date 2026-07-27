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

package service

import (
	"context"
	"math"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// RiskDashboardService assembles the risk dashboard payload from the raw fact
// rows the repository produces.
//
// The pivot helpers below are deliberately pure functions of their arguments —
// no clock, no database, no receiver — so they are directly testable and the
// tests need no fixtures beyond literal fact rows.
type RiskDashboardService interface {
	Summary(ctx context.Context, req domain.RiskDashboardRequest) (domain.RiskDashboardSummary, error)
}

type riskDashboardService struct{ repo repository.RiskDashboardRepository }

// NewRiskDashboardService constructs a RiskDashboardService.
func NewRiskDashboardService(repo repository.RiskDashboardRepository) RiskDashboardService {
	return &riskDashboardService{repo: repo}
}

// statusBucketOrder fixes the x-axis order of each register's status chart.
var statusBucketOrder = []string{"CLOSED", "REMEDIATE", "ACCEPT", "TRANSFER", "VOID"}

func (s *riskDashboardService) Summary(ctx context.Context, req domain.RiskDashboardRequest) (domain.RiskDashboardSummary, error) {
	if req.RegisterID != nil && *req.RegisterID <= 0 {
		return domain.RiskDashboardSummary{}, &apierror.ValidationError{Msg: "registerId must be a positive integer"}
	}
	registerID := req.RegisterID

	counts, err := s.repo.StatusCounts(ctx, registerID)
	if err != nil {
		return domain.RiskDashboardSummary{}, err
	}
	facts, err := s.repo.OpenRiskFacts(ctx, registerID)
	if err != nil {
		return domain.RiskDashboardSummary{}, err
	}
	statusFacts, err := s.repo.RegisterStatusFacts(ctx, registerID)
	if err != nil {
		return domain.RiskDashboardSummary{}, err
	}
	certCounts, err := s.repo.CertTagCounts(ctx, registerID)
	if err != nil {
		return domain.RiskDashboardSummary{}, err
	}
	repeatedRows, err := s.repo.RepeatedComplianceRisks(ctx, registerID)
	if err != nil {
		return domain.RiskDashboardSummary{}, err
	}
	highRisks, err := s.repo.HighRisks(ctx, registerID)
	if err != nil {
		return domain.RiskDashboardSummary{}, err
	}
	if highRisks == nil {
		highRisks = []domain.HighRiskItem{}
	}
	levelOrder, err := s.repo.LevelOrder(ctx)
	if err != nil {
		return domain.RiskDashboardSummary{}, err
	}

	return domain.RiskDashboardSummary{
		Summary:                 *counts,
		TreatmentByRegister:     buildTreatmentByRegister(facts),
		LevelCounts:             buildLevelCounts(facts, levelOrder),
		OrgHeatmap:              buildHeatmap(facts),
		CertDistribution:        buildCertDistribution(certCounts),
		Registers:               buildRegisterBlocks(facts, statusFacts, levelOrder),
		RepeatedComplianceRisks: buildRepeatedRisks(repeatedRows),
		HighRisks:               highRisks,
	}, nil
}

// buildTreatmentByRegister collapses facts into register × treatment counts,
// preserving the repository's register-name ordering.
func buildTreatmentByRegister(facts []domain.OpenRiskFact) []domain.RegisterTreatmentCount {
	type key struct{ register, strategy string }
	counts := map[key]int{}
	var order []key
	for _, f := range facts {
		k := key{f.RegisterName, f.TreatmentStrategy}
		if _, seen := counts[k]; !seen {
			order = append(order, k)
		}
		counts[k] += f.Count
	}
	out := make([]domain.RegisterTreatmentCount, 0, len(order))
	for _, k := range order {
		out = append(out, domain.RegisterTreatmentCount{
			RegisterName:      k.register,
			TreatmentStrategy: k.strategy,
			Count:             counts[k],
		})
	}
	return out
}

// buildLevelCounts collapses facts into per-level totals, ordered by levelOrder
// (severity, highest first — sourced from risk_score so a level added there is
// picked up automatically instead of being silently dropped).
func buildLevelCounts(facts []domain.OpenRiskFact, levelOrder []string) []domain.RiskLevelCount {
	counts := map[string]int{}
	colors := map[string]string{}
	for _, f := range facts {
		counts[f.RiskLevel] += f.Count
		colors[f.RiskLevel] = f.ColorCode
	}
	out := make([]domain.RiskLevelCount, 0, len(levelOrder))
	for _, level := range levelOrder {
		if n, ok := counts[level]; ok {
			out = append(out, domain.RiskLevelCount{RiskLevel: level, ColorCode: colors[level], Count: n})
		}
	}
	return out
}

// buildHeatmap collapses facts into likelihood × impact cell counts. Only
// populated cells are returned; the caller renders the full grid.
func buildHeatmap(facts []domain.OpenRiskFact) []domain.HeatmapCell {
	type key struct{ likelihood, impact int }
	cells := map[key]*domain.HeatmapCell{}
	var order []key
	for _, f := range facts {
		k := key{f.Likelihood, f.Impact}
		if c, ok := cells[k]; ok {
			c.Count += f.Count
			continue
		}
		cells[k] = &domain.HeatmapCell{
			Likelihood: f.Likelihood,
			Impact:     f.Impact,
			RiskLevel:  f.RiskLevel,
			ColorCode:  f.ColorCode,
			Count:      f.Count,
		}
		order = append(order, k)
	}
	out := make([]domain.HeatmapCell, 0, len(order))
	for _, k := range order {
		out = append(out, *cells[k])
	}
	return out
}

// buildCertDistribution converts cert-tag counts into each certification's
// percentage share of its register's tags (segments per register total 100%).
func buildCertDistribution(counts []domain.RegisterCertCount) []domain.RegisterCertShare {
	totals := map[string]int{}
	for _, c := range counts {
		totals[c.RegisterName] += c.Count
	}
	out := make([]domain.RegisterCertShare, 0, len(counts))
	for _, c := range counts {
		pct := float64(c.Count) * 100 / float64(totals[c.RegisterName])
		out = append(out, domain.RegisterCertShare{
			RegisterName: c.RegisterName,
			CertName:     c.CertName,
			Count:        c.Count,
			Percentage:   math.Round(pct*10) / 10,
		})
	}
	return out
}

// buildRegisterBlocks groups facts into one dashboard section per register.
// statusFacts (open + closed) is the superset that determines which registers
// appear and their order; facts (open-only) supplements OpenCount and the
// heatmap, which stay open-scoped.
func buildRegisterBlocks(facts []domain.OpenRiskFact, statusFacts []domain.RegisterStatusFact, levelOrder []string) []domain.RegisterAnalytics {
	blocks := map[int]*domain.RegisterAnalytics{}
	var order []int

	statusGrouped := map[int][]domain.RegisterStatusFact{}
	for _, f := range statusFacts {
		if _, ok := blocks[f.RegisterID]; !ok {
			blocks[f.RegisterID] = &domain.RegisterAnalytics{
				RegisterID:   f.RegisterID,
				RegisterName: f.RegisterName,
			}
			order = append(order, f.RegisterID)
		}
		statusGrouped[f.RegisterID] = append(statusGrouped[f.RegisterID], f)
	}

	grouped := map[int][]domain.OpenRiskFact{}
	for _, f := range facts {
		// An open risk whose register has no status facts cannot happen — the
		// status query is the superset — but guard rather than panic if it does.
		if b, ok := blocks[f.RegisterID]; ok {
			b.OpenCount += f.Count
		}
		grouped[f.RegisterID] = append(grouped[f.RegisterID], f)
	}

	out := make([]domain.RegisterAnalytics, 0, len(order))
	for _, id := range order {
		b := blocks[id]
		b.Heatmap = buildHeatmap(grouped[id])
		b.StatusLevels = buildStatusLevels(statusGrouped[id], levelOrder)
		out = append(out, *b)
	}
	return out
}

// buildStatusLevels collapses one register's status facts into bucket × level
// counts, ordered by statusBucketOrder then levelOrder (severity).
func buildStatusLevels(facts []domain.RegisterStatusFact, levelOrder []string) []domain.RegisterStatusLevelCount {
	type key struct{ bucket, level string }
	counts := map[key]int{}
	colors := map[string]string{}
	for _, f := range facts {
		counts[key{f.Bucket, f.RiskLevel}] += f.Count
		colors[f.RiskLevel] = f.ColorCode
	}
	var out []domain.RegisterStatusLevelCount
	for _, bucket := range statusBucketOrder {
		for _, level := range levelOrder {
			if n, ok := counts[key{bucket, level}]; ok {
				out = append(out, domain.RegisterStatusLevelCount{
					Bucket:    bucket,
					RiskLevel: level,
					ColorCode: colors[level],
					Count:     n,
				})
			}
		}
	}
	return out
}

// buildRepeatedRisks groups per-register occurrences under their shared title,
// preserving the repository's title ordering.
func buildRepeatedRisks(rows []domain.RepeatedRiskRow) []domain.RepeatedComplianceRisk {
	grouped := map[string]*domain.RepeatedComplianceRisk{}
	var order []string
	for _, r := range rows {
		g, ok := grouped[r.RiskTitle]
		if !ok {
			g = &domain.RepeatedComplianceRisk{RiskTitle: r.RiskTitle}
			grouped[r.RiskTitle] = g
			order = append(order, r.RiskTitle)
		}
		g.Occurrences = append(g.Occurrences, domain.RepeatedRiskOccurrence{
			RegisterName: r.RegisterName,
			Status:       r.Status,
			RiskLevel:    r.RiskLevel,
			ColorCode:    r.ColorCode,
		})
	}
	out := make([]domain.RepeatedComplianceRisk, 0, len(order))
	for _, title := range order {
		out = append(out, *grouped[title])
	}
	return out
}

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

package service

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

// trendWindowMonths is the trailing window shown by the trend and level
// distribution charts.
const trendWindowMonths = 12

// agingRisksLimit caps the "Aging Open Risks" table.
const agingRisksLimit = 10

// RiskAnalyticsService assembles the risk analytics summary payload.
type RiskAnalyticsService interface {
	Summary(ctx context.Context, req domain.RiskAnalyticsRequest) (domain.RiskAnalyticsSummary, error)
}

type riskAnalyticsService struct {
	repo repository.RiskAnalyticsRepository
}

// NewRiskAnalyticsService creates a RiskAnalyticsService backed by repo.
func NewRiskAnalyticsService(repo repository.RiskAnalyticsRepository) RiskAnalyticsService {
	return &riskAnalyticsService{repo: repo}
}

// Summary owns the clock and the window constants: the entity decides what
// "the trailing 12 months" means, so every caller sees the same window and none
// has to compute date bounds just to ask for a chart.
func (s *riskAnalyticsService) Summary(ctx context.Context, req domain.RiskAnalyticsRequest) (domain.RiskAnalyticsSummary, error) {
	if req.RegisterID != nil && *req.RegisterID <= 0 {
		return domain.RiskAnalyticsSummary{}, &apierror.ValidationError{Msg: "registerId must be a positive integer"}
	}
	registerID := req.RegisterID
	now := time.Now().UTC()
	monthStart := firstOfMonth(now)
	since := monthStart.AddDate(0, -(trendWindowMonths - 1), 0)
	months := scaffoldMonths(since, trendWindowMonths)

	kpis, err := s.buildKPIs(ctx, registerID, monthStart)
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}

	identified, err := s.repo.IdentifiedTrend(ctx, registerID, dateString(since))
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}
	closed, err := s.repo.ClosedTrend(ctx, registerID, dateString(since))
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}
	levelRows, err := s.repo.LevelDistribution(ctx, registerID, dateString(since))
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}
	levelRef, err := s.repo.LevelReference(ctx)
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}

	var registerShares []domain.RegisterShare
	var identifiedByRegister, closedByRegister []domain.MonthRegisterCount
	if registerID == nil {
		registerShares, err = s.repo.RegisterTotals(ctx)
		if err != nil {
			return domain.RiskAnalyticsSummary{}, err
		}
		identifiedRows, err := s.repo.IdentifiedTrendByRegister(ctx, registerID, dateString(since))
		if err != nil {
			return domain.RiskAnalyticsSummary{}, err
		}
		closedRows, err := s.repo.ClosedTrendByRegister(ctx, registerID, dateString(since))
		if err != nil {
			return domain.RiskAnalyticsSummary{}, err
		}
		identifiedByRegister = buildTrendByRegister(months, identifiedRows)
		closedByRegister = buildTrendByRegister(months, closedRows)
	}

	complianceShares, err := s.repo.ComplianceDistribution(ctx, registerID)
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}
	treatmentShares, err := s.repo.TreatmentMix(ctx, registerID)
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}
	funnel, err := s.repo.WorkflowFunnel(ctx, registerID)
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}
	aging, err := s.repo.AgingRisks(ctx, registerID, agingRisksLimit)
	if err != nil {
		return domain.RiskAnalyticsSummary{}, err
	}
	if aging == nil {
		aging = []domain.AgingRiskItem{}
	}
	if complianceShares == nil {
		complianceShares = []domain.ComplianceShare{}
	}
	if treatmentShares == nil {
		treatmentShares = []domain.TreatmentShare{}
	}
	if funnel == nil {
		funnel = []domain.WorkflowStageCount{}
	}

	return domain.RiskAnalyticsSummary{
		KPIs:                 *kpis,
		Trend:                buildTrend(months, identified, closed),
		LevelDistribution:    buildLevelDistribution(months, levelRows, levelRef),
		IdentifiedByRegister: identifiedByRegister,
		ClosedByRegister:     closedByRegister,
		RegisterShares:       registerShares,
		ComplianceShares:     complianceShares,
		TreatmentShares:      treatmentShares,
		WorkflowFunnel:       funnel,
		AgingRisks:           aging,
	}, nil
}

func (s *riskAnalyticsService) buildKPIs(ctx context.Context, registerID *int, monthStart time.Time) (*domain.RiskAnalyticsKPIs, error) {
	newThisMonth, err := s.repo.NewThisMonthCount(ctx, registerID, dateString(monthStart))
	if err != nil {
		return nil, err
	}
	avgDays, err := s.repo.AvgDaysToClose(ctx, registerID)
	if err != nil {
		return nil, err
	}
	avgScore, err := s.repo.AvgEffectiveScore(ctx, registerID)
	if err != nil {
		return nil, err
	}
	return &domain.RiskAnalyticsKPIs{
		NewRisksThisMonth: newThisMonth,
		AvgDaysToClose:    roundPtr(avgDays),
		AvgEffectiveScore: roundPtr(avgScore),
	}, nil
}

// buildTrend merges identified/closed monthly rows onto a fixed month
// scaffold so every one of the trailing 12 months appears, even at zero.
func buildTrend(months []string, identified []domain.MonthScoreStat, closed []domain.MonthCount) []domain.TrendPoint {
	identifiedByMonth := make(map[string]domain.MonthScoreStat, len(identified))
	for _, m := range identified {
		identifiedByMonth[m.Month] = m
	}
	closedByMonth := make(map[string]int, len(closed))
	for _, c := range closed {
		closedByMonth[c.Month] = c.Count
	}

	out := make([]domain.TrendPoint, 0, len(months))
	for _, month := range months {
		p := domain.TrendPoint{Month: month, ClosedCount: closedByMonth[month]}
		if id, ok := identifiedByMonth[month]; ok {
			p.IdentifiedCount = id.Count
			p.AvgScore = roundPtr(&id.AvgScore)
		}
		out = append(out, p)
	}
	return out
}

// buildLevelDistribution zero-fills every month × level combination so the
// stacked bar chart always renders a full grid across the trailing window.
// The level set and reference color come from levelRef (sourced from
// risk_score), not a hardcoded list, so a level added to risk_score appears
// automatically instead of being silently dropped.
func buildLevelDistribution(months []string, rows []domain.MonthLevelCount, levelRef []domain.RiskLevelRef) []domain.MonthLevelCount {
	type key struct{ month, level string }
	counts := map[key]domain.MonthLevelCount{}
	for _, r := range rows {
		counts[key{r.Month, r.RiskLevel}] = r
	}

	out := make([]domain.MonthLevelCount, 0, len(months)*len(levelRef))
	for _, month := range months {
		for _, level := range levelRef {
			if r, ok := counts[key{month, level.RiskLevel}]; ok {
				out = append(out, r)
				continue
			}
			out = append(out, domain.MonthLevelCount{
				Month:     month,
				RiskLevel: level.RiskLevel,
				ColorCode: level.ColorCode,
				Count:     0,
			})
		}
	}
	return out
}

// buildTrendByRegister zero-fills every month for each register that has at
// least one row somewhere in the window — a register with no activity at all
// in the last trendWindowMonths gets no line, but once a register qualifies,
// every one of its months is present (zero where absent) so its line spans
// the full chart width. Register set is derived from the data, not a fixed
// list, since registers can be added over time.
func buildTrendByRegister(months []string, rows []domain.MonthRegisterCount) []domain.MonthRegisterCount {
	type key struct{ month, register string }
	counts := map[key]int{}
	var registers []string
	seenRegister := map[string]bool{}
	for _, r := range rows {
		counts[key{r.Month, r.RegisterName}] = r.Count
		if !seenRegister[r.RegisterName] {
			seenRegister[r.RegisterName] = true
			registers = append(registers, r.RegisterName)
		}
	}
	sort.Strings(registers)

	out := make([]domain.MonthRegisterCount, 0, len(months)*len(registers))
	for _, register := range registers {
		for _, month := range months {
			out = append(out, domain.MonthRegisterCount{
				Month:        month,
				RegisterName: register,
				Count:        counts[key{month, register}],
			})
		}
	}
	return out
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func dateString(t time.Time) string {
	return t.Format("2006-01-02")
}

// scaffoldMonths returns n consecutive first-of-month date strings starting
// at since (inclusive).
func scaffoldMonths(since time.Time, n int) []string {
	out := make([]string, 0, n)
	m := firstOfMonth(since)
	for i := 0; i < n; i++ {
		out = append(out, dateString(m))
		m = m.AddDate(0, 1, 0)
	}
	return out
}

// roundPtr rounds *v to one decimal place, preserving nil.
func roundPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := math.Round(*v*10) / 10
	return &r
}

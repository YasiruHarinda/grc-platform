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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
)

// trendWindowMonths is the trailing window shown by the trend and level
// distribution charts.
const trendWindowMonths = 12

// agingRisksLimit caps the "Aging Open Risks" table.
const agingRisksLimit = 10

// AnalyticsService assembles the risk analytics summary payload.
type AnalyticsService interface {
	Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error)
}

type analyticsService struct {
	repo repository.AnalyticsRepository
}

// NewAnalyticsService creates an AnalyticsService backed by repo.
func NewAnalyticsService(repo repository.AnalyticsRepository) AnalyticsService {
	return &analyticsService{repo: repo}
}

func (s *analyticsService) Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error) {
	now := time.Now().UTC()
	monthStart := firstOfMonth(now)
	since := monthStart.AddDate(0, -(trendWindowMonths - 1), 0)
	months := scaffoldMonths(since, trendWindowMonths)

	kpis, err := s.buildKPIs(ctx, registerID, monthStart)
	if err != nil {
		return nil, err
	}

	identified, err := s.repo.IdentifiedTrend(ctx, registerID, dateString(since))
	if err != nil {
		return nil, err
	}
	closed, err := s.repo.ClosedTrend(ctx, registerID, dateString(since))
	if err != nil {
		return nil, err
	}
	levelRows, err := s.repo.LevelDistribution(ctx, registerID, dateString(since))
	if err != nil {
		return nil, err
	}
	levelRef, err := s.repo.LevelReference(ctx)
	if err != nil {
		return nil, err
	}

	var registerShares []model.RegisterShare
	var identifiedByRegister, closedByRegister []model.MonthRegisterCount
	if registerID == nil {
		registerShares, err = s.repo.RegisterTotals(ctx)
		if err != nil {
			return nil, err
		}
		identifiedRows, err := s.repo.IdentifiedTrendByRegister(ctx, registerID, dateString(since))
		if err != nil {
			return nil, err
		}
		closedRows, err := s.repo.ClosedTrendByRegister(ctx, registerID, dateString(since))
		if err != nil {
			return nil, err
		}
		identifiedByRegister = buildTrendByRegister(months, identifiedRows)
		closedByRegister = buildTrendByRegister(months, closedRows)
	}

	complianceShares, err := s.repo.ComplianceDistribution(ctx, registerID)
	if err != nil {
		return nil, err
	}
	treatmentShares, err := s.repo.TreatmentMix(ctx, registerID)
	if err != nil {
		return nil, err
	}
	funnel, err := s.repo.WorkflowFunnel(ctx, registerID)
	if err != nil {
		return nil, err
	}
	aging, err := s.repo.AgingRisks(ctx, registerID, agingRisksLimit)
	if err != nil {
		return nil, err
	}
	if aging == nil {
		aging = []model.AgingRiskItem{}
	}
	if complianceShares == nil {
		complianceShares = []model.ComplianceShare{}
	}
	if treatmentShares == nil {
		treatmentShares = []model.TreatmentShare{}
	}
	if funnel == nil {
		funnel = []model.WorkflowStageCount{}
	}

	return &model.AnalyticsSummary{
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

func (s *analyticsService) buildKPIs(ctx context.Context, registerID *int, monthStart time.Time) (*model.AnalyticsKPIs, error) {
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
	return &model.AnalyticsKPIs{
		NewRisksThisMonth: newThisMonth,
		AvgDaysToClose:    roundPtr(avgDays),
		AvgEffectiveScore: roundPtr(avgScore),
	}, nil
}

// buildTrend merges identified/closed monthly rows onto a fixed month
// scaffold so every one of the trailing 12 months appears, even at zero.
func buildTrend(months []string, identified []model.MonthScoreStat, closed []model.MonthCount) []model.TrendPoint {
	identifiedByMonth := make(map[string]model.MonthScoreStat, len(identified))
	for _, m := range identified {
		identifiedByMonth[m.Month] = m
	}
	closedByMonth := make(map[string]int, len(closed))
	for _, c := range closed {
		closedByMonth[c.Month] = c.Count
	}

	out := make([]model.TrendPoint, 0, len(months))
	for _, month := range months {
		p := model.TrendPoint{Month: month, ClosedCount: closedByMonth[month]}
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
func buildLevelDistribution(months []string, rows []model.MonthLevelCount, levelRef []model.RiskLevelRef) []model.MonthLevelCount {
	type key struct{ month, level string }
	counts := map[key]model.MonthLevelCount{}
	for _, r := range rows {
		counts[key{r.Month, r.RiskLevel}] = r
	}

	out := make([]model.MonthLevelCount, 0, len(months)*len(levelRef))
	for _, month := range months {
		for _, level := range levelRef {
			if r, ok := counts[key{month, level.RiskLevel}]; ok {
				out = append(out, r)
				continue
			}
			out = append(out, model.MonthLevelCount{
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
func buildTrendByRegister(months []string, rows []model.MonthRegisterCount) []model.MonthRegisterCount {
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

	out := make([]model.MonthRegisterCount, 0, len(months)*len(registers))
	for _, register := range registers {
		for _, month := range months {
			out = append(out, model.MonthRegisterCount{
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

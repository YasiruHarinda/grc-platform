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

package entity

import (
	"context"
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

// AnalyticsRepository is the entity-backed analytics read.
//
// Like the dashboard, it does not implement repository.AnalyticsRepository:
// those fourteen fact-level queries and the month scaffolding around them now
// live in the entity, which owns the trailing-window definition and returns the
// finished payload.
type AnalyticsRepository interface {
	Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error)
}

type analyticsRepository struct{ c *entityclient.Client }

// NewAnalyticsRepository creates a Compliance Entity-backed analytics read.
func NewAnalyticsRepository(c *entityclient.Client) AnalyticsRepository {
	return &analyticsRepository{c: c}
}

// entAnalytics mirrors the entity's camelCase payload. It exists because
// decoding camelCase into the backend's snake_case tags silently yields empty
// strings — encoding/json matches keys against the tag, and "riskLevel" is not
// a case-insensitive match for "risk_level".
type entAnalytics struct {
	KPIs struct {
		NewRisksThisMonth int      `json:"newRisksThisMonth"`
		AvgDaysToClose    *float64 `json:"avgDaysToClose"`
		AvgEffectiveScore *float64 `json:"avgEffectiveScore"`
	} `json:"kpis"`
	Trend []struct {
		Month           string   `json:"month"`
		IdentifiedCount int      `json:"identifiedCount"`
		ClosedCount     int      `json:"closedCount"`
		AvgScore        *float64 `json:"avgScore"`
	} `json:"trend"`
	LevelDistribution []struct {
		Month     string `json:"month"`
		RiskLevel string `json:"riskLevel"`
		ColorCode string `json:"colorCode"`
		Count     int    `json:"count"`
	} `json:"levelDistribution"`
	IdentifiedByRegister []entMonthRegisterCount `json:"identifiedByRegister"`
	ClosedByRegister     []entMonthRegisterCount `json:"closedByRegister"`
	RegisterShares       []struct {
		RegisterName string `json:"registerName"`
		Count        int    `json:"count"`
	} `json:"registerShares"`
	ComplianceShares []struct {
		ComplianceName string `json:"complianceName"`
		Count          int    `json:"count"`
	} `json:"complianceDistribution"`
	TreatmentShares []struct {
		TreatmentStrategy string `json:"treatmentStrategy"`
		Count             int    `json:"count"`
	} `json:"treatmentMix"`
	WorkflowFunnel []struct {
		WorkflowStatus string `json:"workflowStatus"`
		Count          int    `json:"count"`
	} `json:"workflowFunnel"`
	AgingRisks []struct {
		ID             int     `json:"id"`
		RiskCode       string  `json:"riskCode"`
		RiskTitle      string  `json:"riskTitle"`
		RegisterName   string  `json:"registerName"`
		OwnerName      string  `json:"ownerName"`
		RiskLevel      string  `json:"riskLevel"`
		ColorCode      string  `json:"colorCode"`
		IdentifiedDate *string `json:"identifiedDate"`
		AgeDays        int     `json:"ageDays"`
	} `json:"agingRisks"`
}

type entMonthRegisterCount struct {
	Month        string `json:"month"`
	RegisterName string `json:"registerName"`
	Count        int    `json:"count"`
}

// Summary fetches the assembled analytics payload.
//
// IdentifiedByRegister, ClosedByRegister and RegisterShares are left nil rather
// than empty when a register filter is applied: the MySQL service omitted them
// entirely in that case, and the frontend distinguishes null from [].
func (r *analyticsRepository) Summary(ctx context.Context, registerID *int) (*model.AnalyticsSummary, error) {
	body := map[string]any{"registerId": registerID}
	var e entAnalytics
	if err := r.c.Post(ctx, "/risk/analytics/search", body, &e); err != nil {
		return nil, fmt.Errorf("risk analytics: %w", err)
	}

	out := &model.AnalyticsSummary{
		KPIs: model.AnalyticsKPIs{
			NewRisksThisMonth: e.KPIs.NewRisksThisMonth,
			AvgDaysToClose:    e.KPIs.AvgDaysToClose,
			AvgEffectiveScore: e.KPIs.AvgEffectiveScore,
		},
		Trend:             make([]model.TrendPoint, 0, len(e.Trend)),
		LevelDistribution: make([]model.MonthLevelCount, 0, len(e.LevelDistribution)),
		ComplianceShares:  make([]model.ComplianceShare, 0, len(e.ComplianceShares)),
		TreatmentShares:   make([]model.TreatmentShare, 0, len(e.TreatmentShares)),
		WorkflowFunnel:    make([]model.WorkflowStageCount, 0, len(e.WorkflowFunnel)),
		AgingRisks:        make([]model.AgingRiskItem, 0, len(e.AgingRisks)),
	}

	for _, t := range e.Trend {
		out.Trend = append(out.Trend, model.TrendPoint{
			Month: t.Month, IdentifiedCount: t.IdentifiedCount,
			ClosedCount: t.ClosedCount, AvgScore: t.AvgScore,
		})
	}
	for _, l := range e.LevelDistribution {
		out.LevelDistribution = append(out.LevelDistribution, model.MonthLevelCount{
			Month: l.Month, RiskLevel: l.RiskLevel, ColorCode: l.ColorCode, Count: l.Count,
		})
	}
	if e.IdentifiedByRegister != nil {
		out.IdentifiedByRegister = mapMonthRegisterCounts(e.IdentifiedByRegister)
	}
	if e.ClosedByRegister != nil {
		out.ClosedByRegister = mapMonthRegisterCounts(e.ClosedByRegister)
	}
	if e.RegisterShares != nil {
		out.RegisterShares = make([]model.RegisterShare, 0, len(e.RegisterShares))
		for _, s := range e.RegisterShares {
			out.RegisterShares = append(out.RegisterShares, model.RegisterShare{
				RegisterName: s.RegisterName, Count: s.Count,
			})
		}
	}
	for _, c := range e.ComplianceShares {
		out.ComplianceShares = append(out.ComplianceShares, model.ComplianceShare{
			ComplianceName: c.ComplianceName, Count: c.Count,
		})
	}
	for _, t := range e.TreatmentShares {
		out.TreatmentShares = append(out.TreatmentShares, model.TreatmentShare{
			TreatmentStrategy: t.TreatmentStrategy, Count: t.Count,
		})
	}
	for _, f := range e.WorkflowFunnel {
		out.WorkflowFunnel = append(out.WorkflowFunnel, model.WorkflowStageCount{
			WorkflowStatus: f.WorkflowStatus, Count: f.Count,
		})
	}
	for _, a := range e.AgingRisks {
		out.AgingRisks = append(out.AgingRisks, model.AgingRiskItem{
			ID: a.ID, RiskCode: a.RiskCode, RiskTitle: a.RiskTitle,
			RegisterName: a.RegisterName, OwnerName: a.OwnerName,
			RiskLevel: a.RiskLevel, ColorCode: a.ColorCode,
			IdentifiedDate: dateOnlyPtrToRFC3339(a.IdentifiedDate),
			AgeDays:        a.AgeDays,
		})
	}
	return out, nil
}

func mapMonthRegisterCounts(in []entMonthRegisterCount) []model.MonthRegisterCount {
	out := make([]model.MonthRegisterCount, 0, len(in))
	for _, m := range in {
		out = append(out, model.MonthRegisterCount{
			Month: m.Month, RegisterName: m.RegisterName, Count: m.Count,
		})
	}
	return out
}

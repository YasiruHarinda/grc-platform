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

// DashboardRepository is the entity-backed dashboard read.
//
// It deliberately does not implement repository.DashboardRepository: that
// interface exposes seven fact-level queries the backend service pivoted
// itself. The entity now does both the aggregation and the pivoting and returns
// the finished payload, so only this one call remains — mirroring the audit
// module, whose dashboard service is likewise a passthrough.
type DashboardRepository interface {
	Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error)
}

type dashboardRepository struct{ c *entityclient.Client }

// NewDashboardRepository creates a Compliance Entity-backed dashboard read.
func NewDashboardRepository(c *entityclient.Client) DashboardRepository {
	return &dashboardRepository{c: c}
}

// The types below mirror the entity's camelCase payload and exist purely to be
// mapped onto the backend's snake_case models.
//
// Decoding the entity's response straight into model.DashboardSummary looks
// like it would work and does not: encoding/json matches an incoming key
// against the field's *tag*, and "registerName" is not a case-insensitive match
// for "register_name". Every string field would come back empty while numeric
// fields whose names happen to coincide (count) would populate — a dashboard of
// blank labels rather than an error.
type entDashboard struct {
	Summary struct {
		Total   int `json:"total"`
		Open    int `json:"open"`
		Closed  int `json:"closed"`
		Overdue int `json:"overdue"`
	} `json:"summary"`
	TreatmentByRegister []struct {
		RegisterName      string `json:"registerName"`
		TreatmentStrategy string `json:"treatmentStrategy"`
		Count             int    `json:"count"`
	} `json:"treatmentByRegister"`
	LevelCounts      []entLevelCount  `json:"levelCounts"`
	OrgHeatmap       []entHeatmapCell `json:"orgHeatmap"`
	CertDistribution []struct {
		RegisterName string  `json:"registerName"`
		CertName     string  `json:"certName"`
		Count        int     `json:"count"`
		Percentage   float64 `json:"percentage"`
	} `json:"certDistribution"`
	Registers []struct {
		RegisterID   int              `json:"registerId"`
		RegisterName string           `json:"registerName"`
		OpenCount    int              `json:"openCount"`
		Heatmap      []entHeatmapCell `json:"heatmap"`
		StatusLevels []struct {
			Bucket    string `json:"bucket"`
			RiskLevel string `json:"riskLevel"`
			ColorCode string `json:"colorCode"`
			Count     int    `json:"count"`
		} `json:"statusLevels"`
	} `json:"registers"`
	RepeatedComplianceRisks []struct {
		RiskTitle   string `json:"riskTitle"`
		Occurrences []struct {
			RegisterName string `json:"registerName"`
			Status       string `json:"status"`
			RiskLevel    string `json:"riskLevel"`
			ColorCode    string `json:"colorCode"`
		} `json:"occurrences"`
	} `json:"repeatedComplianceRisks"`
	HighRisks []struct {
		ID                 int     `json:"id"`
		RiskCode           string  `json:"riskCode"`
		RiskTitle          string  `json:"riskTitle"`
		RegisterName       string  `json:"registerName"`
		OwnerName          string  `json:"ownerName"`
		IdentifiedDate     *string `json:"identifiedDate"`
		TreatmentStrategy  *string `json:"treatmentStrategy"`
		ImplementationDate *string `json:"implementationDate"`
	} `json:"highRisks"`
}

type entLevelCount struct {
	RiskLevel string `json:"riskLevel"`
	ColorCode string `json:"colorCode"`
	Count     int    `json:"count"`
}

type entHeatmapCell struct {
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskLevel  string `json:"riskLevel"`
	ColorCode  string `json:"colorCode"`
	Count      int    `json:"count"`
}

func (c entHeatmapCell) toModel() model.HeatmapCell {
	return model.HeatmapCell{
		Likelihood: c.Likelihood,
		Impact:     c.Impact,
		RiskLevel:  c.RiskLevel,
		ColorCode:  c.ColorCode,
		Count:      c.Count,
	}
}

// Summary fetches the assembled dashboard. Slices are initialised even when
// empty so the JSON the backend serves carries [] rather than null, matching
// what the MySQL-backed service produced.
func (r *dashboardRepository) Summary(ctx context.Context, registerID *int) (*model.DashboardSummary, error) {
	body := map[string]any{"registerId": registerID}
	var e entDashboard
	if err := r.c.Post(ctx, "/risk/dashboard/search", body, &e); err != nil {
		return nil, fmt.Errorf("risk dashboard: %w", err)
	}

	out := &model.DashboardSummary{
		Summary: model.RiskStatusSummary{
			Total:   e.Summary.Total,
			Open:    e.Summary.Open,
			Closed:  e.Summary.Closed,
			Overdue: e.Summary.Overdue,
		},
		TreatmentByRegister:     make([]model.RegisterTreatmentCount, 0, len(e.TreatmentByRegister)),
		LevelCounts:             make([]model.RiskLevelCount, 0, len(e.LevelCounts)),
		OrgHeatmap:              make([]model.HeatmapCell, 0, len(e.OrgHeatmap)),
		CertDistribution:        make([]model.RegisterCertShare, 0, len(e.CertDistribution)),
		Registers:               make([]model.RegisterAnalytics, 0, len(e.Registers)),
		RepeatedComplianceRisks: make([]model.RepeatedComplianceRisk, 0, len(e.RepeatedComplianceRisks)),
		HighRisks:               make([]model.HighRiskItem, 0, len(e.HighRisks)),
	}

	for _, t := range e.TreatmentByRegister {
		out.TreatmentByRegister = append(out.TreatmentByRegister, model.RegisterTreatmentCount{
			RegisterName: t.RegisterName, TreatmentStrategy: t.TreatmentStrategy, Count: t.Count,
		})
	}
	for _, l := range e.LevelCounts {
		out.LevelCounts = append(out.LevelCounts, model.RiskLevelCount{
			RiskLevel: l.RiskLevel, ColorCode: l.ColorCode, Count: l.Count,
		})
	}
	for _, c := range e.OrgHeatmap {
		out.OrgHeatmap = append(out.OrgHeatmap, c.toModel())
	}
	for _, c := range e.CertDistribution {
		out.CertDistribution = append(out.CertDistribution, model.RegisterCertShare{
			RegisterName: c.RegisterName, CertName: c.CertName,
			Count: c.Count, Percentage: c.Percentage,
		})
	}
	for _, reg := range e.Registers {
		block := model.RegisterAnalytics{
			RegisterID:   reg.RegisterID,
			RegisterName: reg.RegisterName,
			OpenCount:    reg.OpenCount,
			Heatmap:      make([]model.HeatmapCell, 0, len(reg.Heatmap)),
			StatusLevels: make([]model.RegisterStatusLevelCount, 0, len(reg.StatusLevels)),
		}
		for _, c := range reg.Heatmap {
			block.Heatmap = append(block.Heatmap, c.toModel())
		}
		for _, sl := range reg.StatusLevels {
			block.StatusLevels = append(block.StatusLevels, model.RegisterStatusLevelCount{
				Bucket: sl.Bucket, RiskLevel: sl.RiskLevel, ColorCode: sl.ColorCode, Count: sl.Count,
			})
		}
		out.Registers = append(out.Registers, block)
	}
	for _, rr := range e.RepeatedComplianceRisks {
		item := model.RepeatedComplianceRisk{
			RiskTitle:   rr.RiskTitle,
			Occurrences: make([]model.RepeatedRiskOccurrence, 0, len(rr.Occurrences)),
		}
		for _, o := range rr.Occurrences {
			item.Occurrences = append(item.Occurrences, model.RepeatedRiskOccurrence{
				RegisterName: o.RegisterName, Status: o.Status,
				RiskLevel: o.RiskLevel, ColorCode: o.ColorCode,
			})
		}
		out.RepeatedComplianceRisks = append(out.RepeatedComplianceRisks, item)
	}
	for _, h := range e.HighRisks {
		out.HighRisks = append(out.HighRisks, model.HighRiskItem{
			ID: h.ID, RiskCode: h.RiskCode, RiskTitle: h.RiskTitle,
			RegisterName: h.RegisterName, OwnerName: h.OwnerName,
			IdentifiedDate:     dateOnlyPtrToRFC3339(h.IdentifiedDate),
			TreatmentStrategy:  h.TreatmentStrategy,
			ImplementationDate: dateOnlyPtrToRFC3339(h.ImplementationDate),
		})
	}
	return out, nil
}

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

package model

// AnalyticsSummary is the full payload for GET /api/v1/risks/analytics/summary.
// It is the "over time" and cross-cutting companion to DashboardSummary's
// point-in-time snapshot — charts here intentionally avoid duplicating what
// the dashboard already shows.
//
// Scope rules:
//   - CANCELLED risks are excluded everywhere.
//   - "Open" = any status other than CLOSED (and not CANCELLED); "Closed" = CLOSED.
//   - Risk level/score use the effective residual score: the latest
//     risk_assessment score when one exists, else the gross score.
//   - RegisterID, when non-nil, scopes every field to that register except
//     RegisterShares, IdentifiedByRegister, and ClosedByRegister, which are
//     cross-register comparisons and are only populated when RegisterID is
//     nil (i.e. the "All registers" view).
type AnalyticsSummary struct {
	KPIs                 AnalyticsKPIs        `json:"kpis"`
	Trend                []TrendPoint         `json:"trend"`
	LevelDistribution    []MonthLevelCount    `json:"level_distribution"`
	IdentifiedByRegister []MonthRegisterCount `json:"identified_by_register"`
	ClosedByRegister     []MonthRegisterCount `json:"closed_by_register"`
	RegisterShares       []RegisterShare      `json:"register_shares"`
	ComplianceShares     []ComplianceShare    `json:"compliance_distribution"`
	TreatmentShares      []TreatmentShare     `json:"treatment_mix"`
	WorkflowFunnel       []WorkflowStageCount `json:"workflow_funnel"`
	AgingRisks           []AgingRiskItem      `json:"aging_risks"`
}

// AnalyticsKPIs backs the three "Key Risk Metrics" tiles.
// AvgDaysToClose and AvgEffectiveScore are nil when there is no qualifying
// risk to average (no closed risks / no open risks, respectively) so the
// frontend can render "—" instead of a misleading zero.
// Total/Open/Overdue counts live on the Dashboard's RiskStatusSummary instead.
type AnalyticsKPIs struct {
	NewRisksThisMonth int      `json:"new_risks_this_month"`
	AvgDaysToClose    *float64 `json:"avg_days_to_close"`
	AvgEffectiveScore *float64 `json:"avg_effective_score"`
}

// TrendPoint is one month of the "Risk Trend Over Time" combo chart: bars for
// identified vs. closed counts, line for average effective score. Always
// covers the trailing 12 months, including months with zero activity.
type TrendPoint struct {
	Month           string   `json:"month"` // first-of-month date, YYYY-MM-01
	IdentifiedCount int      `json:"identified_count"`
	ClosedCount     int      `json:"closed_count"`
	AvgScore        *float64 `json:"avg_score"` // nil when no risks were identified that month
}

// MonthLevelCount is one stacked segment of the "Risk Level Distribution Over
// Time" chart (x = month, stack = one segment per level defined in
// risk_score). Always covers the trailing 12 months × every level, zero-filled
// where absent.
type MonthLevelCount struct {
	Month     string `json:"month"`
	RiskLevel string `json:"risk_level"`
	ColorCode string `json:"color_code"`
	Count     int    `json:"count"`
}

// RiskLevelRef is one distinct risk level defined in the risk_score reference
// table, ordered by severity (highest first), with its reference color. Used
// to drive the emitted level set for level-based charts instead of a
// hardcoded list, so a level added to risk_score is picked up automatically.
type RiskLevelRef struct {
	RiskLevel string
	ColorCode string
}

// MonthRegisterCount is one line-point of the "Risks Identified by Source
// Register" / "Risks Closed by Source Register" trend charts (x = month,
// one line per register). Only registers with at least one identified/closed
// risk in the trailing 12-month window get a line at all; once a register
// has one, every month is zero-filled so its line spans the full window.
type MonthRegisterCount struct {
	Month        string `json:"month"`
	RegisterName string `json:"register_name"`
	Count        int    `json:"count"`
}

// RegisterShare is one slice of the "Risks by Register" comparison donut:
// total risk count (open + closed, all-time) per register. Only populated
// when the page's register filter is "All".
type RegisterShare struct {
	RegisterName string `json:"register_name"`
	Count        int    `json:"count"`
}

// ComplianceShare is one slice of the org-wide "Compliance Reference
// Distribution" donut: total risk count tagged per compliance framework,
// all registers combined (or scoped to one register when filtered).
type ComplianceShare struct {
	ComplianceName string `json:"compliance_name"`
	Count          int    `json:"count"`
}

// TreatmentShare is one slice of the org-wide "Risk Treatment Strategies"
// chart: open risk count per treatment strategy.
type TreatmentShare struct {
	TreatmentStrategy string `json:"treatment_strategy"`
	Count             int    `json:"count"`
}

// WorkflowStageCount is one bar of the "Workflow Status Funnel" chart.
type WorkflowStageCount struct {
	WorkflowStatus string `json:"workflow_status"`
	Count          int    `json:"count"`
}

// AgingRiskItem is one row of the "Aging Open Risks" table: an open risk
// ranked by days since it was identified.
type AgingRiskItem struct {
	ID             int     `json:"id"`
	RiskCode       string  `json:"risk_code"`
	RiskTitle      string  `json:"risk_title"`
	RegisterName   string  `json:"register_name"`
	OwnerName      string  `json:"owner_name"`
	RiskLevel      string  `json:"risk_level"`
	ColorCode      string  `json:"color_code"`
	IdentifiedDate *string `json:"identified_date"`
	AgeDays        int     `json:"age_days"`
}

// MonthCount is a raw repository row: one month's count of some event.
type MonthCount struct {
	Month string
	Count int
}

// MonthScoreStat is a raw repository row: one month's identified-risk count
// and the average effective score of risks identified that month.
type MonthScoreStat struct {
	Month    string
	Count    int
	AvgScore float64
}

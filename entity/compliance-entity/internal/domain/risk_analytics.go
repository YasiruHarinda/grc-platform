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

package domain

// Risk analytics payload. Trend charts cover a trailing 12-month window ending
// with the current month, zero-filled so every month renders even when nothing
// happened in it. An optional registerId scopes the whole payload.

// RiskAnalyticsRequest is the payload for POST /risk/analytics/search.
// RegisterID nil means every register.
type RiskAnalyticsRequest struct {
	RegisterID *int `json:"registerId"`
}

// RiskAnalyticsSummary is the assembled analytics page.
type RiskAnalyticsSummary struct {
	KPIs                 RiskAnalyticsKPIs    `json:"kpis"`
	Trend                []TrendPoint         `json:"trend"`
	LevelDistribution    []MonthLevelCount    `json:"levelDistribution"`
	IdentifiedByRegister []MonthRegisterCount `json:"identifiedByRegister"`
	ClosedByRegister     []MonthRegisterCount `json:"closedByRegister"`
	RegisterShares       []RegisterShare      `json:"registerShares"`
	ComplianceShares     []ComplianceShare    `json:"complianceDistribution"`
	TreatmentShares      []TreatmentShare     `json:"treatmentMix"`
	WorkflowFunnel       []WorkflowStageCount `json:"workflowFunnel"`
	AgingRisks           []AgingRiskItem      `json:"agingRisks"`
}

// RiskAnalyticsKPIs backs the key-metric tiles. AvgDaysToClose and
// AvgEffectiveScore are null when there is nothing to average — no closed risks
// and no open risks respectively — so a caller can render a dash rather than a
// misleading zero.
type RiskAnalyticsKPIs struct {
	NewRisksThisMonth int      `json:"newRisksThisMonth"`
	AvgDaysToClose    *float64 `json:"avgDaysToClose"`
	AvgEffectiveScore *float64 `json:"avgEffectiveScore"`
}

// TrendPoint is one month of the identified/closed trend. AvgScore is null when
// no risk was identified that month.
type TrendPoint struct {
	Month           string   `json:"month"` // first-of-month, YYYY-MM-01
	IdentifiedCount int      `json:"identifiedCount"`
	ClosedCount     int      `json:"closedCount"`
	AvgScore        *float64 `json:"avgScore"`
}

// MonthLevelCount is one segment of the level-distribution chart. The full
// month × level grid is emitted, zero-filled.
type MonthLevelCount struct {
	Month     string `json:"month"`
	RiskLevel string `json:"riskLevel"`
	ColorCode string `json:"colorCode"`
	Count     int    `json:"count"`
}

// MonthRegisterCount is one point of a per-register trend line. A register with
// no activity in the window gets no line at all; once it qualifies, every month
// is present so its line spans the full width.
type MonthRegisterCount struct {
	Month        string `json:"month"`
	RegisterName string `json:"registerName"`
	Count        int    `json:"count"`
}

// RegisterShare is one slice of the risks-by-register comparison, all-time.
// Only populated when no register filter is applied.
type RegisterShare struct {
	RegisterName string `json:"registerName"`
	Count        int    `json:"count"`
}

// ComplianceShare is total risk count tagged per compliance framework.
type ComplianceShare struct {
	ComplianceName string `json:"complianceName"`
	Count          int    `json:"count"`
}

// TreatmentShare is open risk count per treatment strategy.
type TreatmentShare struct {
	TreatmentStrategy string `json:"treatmentStrategy"`
	Count             int    `json:"count"`
}

// WorkflowStageCount is one bar of the workflow funnel.
type WorkflowStageCount struct {
	WorkflowStatus string `json:"workflowStatus"`
	Count          int    `json:"count"`
}

// AgingRiskItem is one row of the aging open risks table, oldest first.
type AgingRiskItem struct {
	ID             int     `json:"id"`
	RiskCode       string  `json:"riskCode"`
	RiskTitle      string  `json:"riskTitle"`
	RegisterName   string  `json:"registerName"`
	OwnerName      string  `json:"ownerName"`
	RiskLevel      string  `json:"riskLevel"`
	ColorCode      string  `json:"colorCode"`
	IdentifiedDate *string `json:"identifiedDate"`
	AgeDays        int     `json:"ageDays"`
}

// ── Raw fact rows ────────────────────────────────────────────────────────────
// Consumed only by the service's scaffolding helpers; not serialised.

// RiskLevelRef is one distinct level defined in risk_score, ordered by severity
// (highest first), with its reference colour. Level-based charts derive their
// level set from this rather than a hardcoded list, so a level added to
// risk_score is picked up automatically instead of being silently dropped.
type RiskLevelRef struct {
	RiskLevel string
	ColorCode string
}

// MonthCount is one month's count of some event.
type MonthCount struct {
	Month string
	Count int
}

// MonthScoreStat is one month's identified-risk count and their average
// effective score.
type MonthScoreStat struct {
	Month    string
	Count    int
	AvgScore float64
}

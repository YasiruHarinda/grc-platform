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

// Risk dashboard payload. The service aggregates the raw fact rows below into
// the chart-shaped structures above them, so a caller renders the page from one
// response instead of issuing seven queries and pivoting them itself.
//
// Every query excludes CANCELLED risks; "open" means any status other than
// CLOSED. An optional registerId scopes the whole payload to one register.

// RiskDashboardRequest is the payload for POST /risk/dashboard/search.
// RegisterID nil means every register.
type RiskDashboardRequest struct {
	RegisterID *int `json:"registerId"`
}

// RiskDashboardSummary is the assembled dashboard.
type RiskDashboardSummary struct {
	Summary                 RiskStatusSummary        `json:"summary"`
	TreatmentByRegister     []RegisterTreatmentCount `json:"treatmentByRegister"`
	LevelCounts             []RiskLevelCount         `json:"levelCounts"`
	OrgHeatmap              []HeatmapCell            `json:"orgHeatmap"`
	CertDistribution        []RegisterCertShare      `json:"certDistribution"`
	Registers               []RegisterAnalytics      `json:"registers"`
	RepeatedComplianceRisks []RepeatedComplianceRisk `json:"repeatedComplianceRisks"`
	HighRisks               []HighRiskItem           `json:"highRisks"`
}

// RiskStatusSummary backs the summary cards and the open/closed split.
type RiskStatusSummary struct {
	Total   int `json:"total"`
	Open    int `json:"open"`
	Closed  int `json:"closed"`
	Overdue int `json:"overdue"`
}

// RegisterTreatmentCount is one stacked segment of the treatment-strategy chart.
type RegisterTreatmentCount struct {
	RegisterName      string `json:"registerName"`
	TreatmentStrategy string `json:"treatmentStrategy"`
	Count             int    `json:"count"`
}

// RiskLevelCount is one bar of the count-by-level chart.
type RiskLevelCount struct {
	RiskLevel string `json:"riskLevel"`
	ColorCode string `json:"colorCode"`
	Count     int    `json:"count"`
}

// HeatmapCell is one populated likelihood × impact cell. Empty cells are
// omitted; the caller draws the full grid.
type HeatmapCell struct {
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskLevel  string `json:"riskLevel"`
	ColorCode  string `json:"colorCode"`
	Count      int    `json:"count"`
}

// RegisterCertShare is one certification's share of its register's tags.
// Percentages within a register total 100.
type RegisterCertShare struct {
	RegisterName string  `json:"registerName"`
	CertName     string  `json:"certName"`
	Count        int     `json:"count"`
	Percentage   float64 `json:"percentage"`
}

// RegisterStatusLevelCount is one bar of a register's status chart.
type RegisterStatusLevelCount struct {
	Bucket    string `json:"bucket"`
	RiskLevel string `json:"riskLevel"`
	ColorCode string `json:"colorCode"`
	Count     int    `json:"count"`
}

// RegisterAnalytics is one register's block of the dashboard.
type RegisterAnalytics struct {
	RegisterID   int                        `json:"registerId"`
	RegisterName string                     `json:"registerName"`
	OpenCount    int                        `json:"openCount"`
	Heatmap      []HeatmapCell              `json:"heatmap"`
	StatusLevels []RegisterStatusLevelCount `json:"statusLevels"`
}

// RepeatedComplianceRisk is one cert-tagged risk title occurring in two or more
// registers, with where it occurs.
type RepeatedComplianceRisk struct {
	RiskTitle   string                   `json:"riskTitle"`
	Occurrences []RepeatedRiskOccurrence `json:"occurrences"`
}

// RepeatedRiskOccurrence is one register's instance of a repeated risk.
type RepeatedRiskOccurrence struct {
	RegisterName string `json:"registerName"`
	Status       string `json:"status"`
	RiskLevel    string `json:"riskLevel"`
	ColorCode    string `json:"colorCode"`
}

// HighRiskItem is one row of the high-risk table, oldest identified first.
type HighRiskItem struct {
	ID                 int     `json:"id"`
	RiskCode           string  `json:"riskCode"`
	RiskTitle          string  `json:"riskTitle"`
	RegisterName       string  `json:"registerName"`
	OwnerName          string  `json:"ownerName"`
	IdentifiedDate     *string `json:"identifiedDate"`
	TreatmentStrategy  *string `json:"treatmentStrategy"`
	ImplementationDate *string `json:"implementationDate"`
}

// ── Raw fact rows ────────────────────────────────────────────────────────────
// Grouped counts straight from SQL, consumed only by the service's pivots.
// They are not serialised.

// OpenRiskFact is open risks grouped by register × score cell × treatment.
type OpenRiskFact struct {
	RegisterID        int
	RegisterName      string
	Likelihood        int
	Impact            int
	RiskLevel         string
	ColorCode         string
	TreatmentStrategy string
	Count             int
}

// RegisterCertCount is open cert-tag occurrences per register × certification.
type RegisterCertCount struct {
	RegisterName string
	CertName     string
	Count        int
}

// RegisterStatusFact is every non-cancelled risk grouped by register × level ×
// status bucket, where the bucket is CLOSED or the treatment strategy.
type RegisterStatusFact struct {
	RegisterID   int
	RegisterName string
	RiskLevel    string
	ColorCode    string
	Bucket       string
	Count        int
}

// RepeatedRiskRow is one occurrence of a repeated cert-tagged risk title.
type RepeatedRiskRow struct {
	RiskTitle    string
	RegisterName string
	Status       string
	RiskLevel    string
	ColorCode    string
}

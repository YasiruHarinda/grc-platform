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

// DashboardSummary is the full payload for GET /api/v1/risks/dashboard.
// Everything the risk dashboard renders comes from this single response.
//
// Scope rules shared by all fields:
//   - CANCELLED risks are excluded everywhere.
//   - "Open" = any status other than CLOSED; "Closed" = CLOSED.
//   - Risk level/likelihood/impact use the effective residual score: the
//     latest risk_assessment score when one exists, else the gross score.
//   - Every field is open-risks-only EXCEPT RegisterAnalytics.StatusLevels,
//     which also includes closed risks (see its own doc comment).
type DashboardSummary struct {
	Summary                 RiskStatusSummary        `json:"summary"`
	TreatmentByRegister     []RegisterTreatmentCount `json:"treatment_by_register"`
	LevelCounts             []RiskLevelCount         `json:"level_counts"`
	OrgHeatmap              []HeatmapCell            `json:"org_heatmap"`
	CertDistribution        []RegisterCertShare      `json:"cert_distribution"`
	Registers               []RegisterAnalytics      `json:"registers"`
	RepeatedComplianceRisks []RepeatedComplianceRisk `json:"repeated_compliance_risks"`
	HighRisks               []HighRiskItem           `json:"high_risks"`
}

// RiskStatusSummary backs the summary cards and the open/closed pie chart.
type RiskStatusSummary struct {
	Total   int `json:"total"`
	Open    int `json:"open"`
	Closed  int `json:"closed"`
	Overdue int `json:"overdue"`
}

// RegisterTreatmentCount is one stacked segment of the
// "Risk Treatment Strategy on Open Risks" chart (x = BU/register).
type RegisterTreatmentCount struct {
	RegisterName      string `json:"register_name"`
	TreatmentStrategy string `json:"treatment_strategy"`
	Count             int    `json:"count"`
}

// RiskLevelCount is one bar of the "Count vs. Risk Level" chart.
type RiskLevelCount struct {
	RiskLevel string `json:"risk_level"`
	ColorCode string `json:"color_code"`
	Count     int    `json:"count"`
}

// HeatmapCell is one cell of a 3×3 likelihood × impact heatmap.
type HeatmapCell struct {
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskLevel  string `json:"risk_level"`
	ColorCode  string `json:"color_code"`
	Count      int    `json:"count"`
}

// RegisterCertShare is one segment of the 100%-stacked
// "Number of Open Risks against Compliance Certifications" chart.
// Percentage is the cert's share of all cert tags on the register's open
// risks (tags, not risks, so a register's segments always total 100%).
type RegisterCertShare struct {
	RegisterName string  `json:"register_name"`
	CertName     string  `json:"cert_name"`
	Count        int     `json:"count"`
	Percentage   float64 `json:"percentage"`
}

// RegisterStatusLevelCount is one stacked segment of a register's status
// chart: x = status bucket (CLOSED, or an open risk's treatment strategy),
// legend = effective residual level.
type RegisterStatusLevelCount struct {
	Bucket    string `json:"bucket"`
	RiskLevel string `json:"risk_level"`
	ColorCode string `json:"color_code"`
	Count     int    `json:"count"`
}

// RegisterAnalytics is one per-register dashboard section. Registers with no
// risks at all (open or closed) are omitted from the payload.
type RegisterAnalytics struct {
	RegisterID   int                        `json:"register_id"`
	RegisterName string                     `json:"register_name"`
	OpenCount    int                        `json:"open_count"`
	Heatmap      []HeatmapCell              `json:"heatmap"`
	StatusLevels []RegisterStatusLevelCount `json:"status_levels"`
}

// RepeatedComplianceRisk is one row group of the "Repeated Risks Potentially
// impacting Compliance Certs" table: a cert-tagged risk title that appears in
// two or more source registers, with one occurrence per register.
type RepeatedComplianceRisk struct {
	RiskTitle   string                   `json:"risk_title"`
	Occurrences []RepeatedRiskOccurrence `json:"occurrences"`
}

// RepeatedRiskOccurrence is one register's instance of a repeated risk.
// Status is the simplified OPEN/CLOSED value shown in the table.
type RepeatedRiskOccurrence struct {
	RegisterName string `json:"register_name"`
	Status       string `json:"status"`
	RiskLevel    string `json:"risk_level"`
	ColorCode    string `json:"color_code"`
}

// HighRiskItem is one row of the "High Severity Open Risks" table: an open
// risk whose effective residual level is HIGH.
type HighRiskItem struct {
	ID                 int     `json:"id"`
	RiskCode           string  `json:"risk_code"`
	RiskTitle          string  `json:"risk_title"`
	RegisterName       string  `json:"register_name"`
	OwnerName          string  `json:"owner_name"`
	IdentifiedDate     *string `json:"identified_date"`
	TreatmentStrategy  *string `json:"treatment_strategy"`
	ImplementationDate *string `json:"implementation_date"`
}

// OpenRiskFact is one aggregated repository row of open risks grouped by
// register × effective score cell × treatment strategy. The dashboard service
// composes the treatment, level, and heatmap charts from these rows.
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

// RegisterCertCount is one repository row: open cert-tag occurrences per
// register × certification.
type RegisterCertCount struct {
	RegisterName string
	CertName     string
	Count        int
}

// RegisterStatusFact is one aggregated repository row of every non-cancelled
// risk (open and closed) grouped by register × effective residual level ×
// status bucket. Bucket is "CLOSED" for closed risks, else the risk's own
// treatment strategy (REMEDIATE/ACCEPT/TRANSFER/VOID). The dashboard service
// composes each register's status chart from these rows.
type RegisterStatusFact struct {
	RegisterID   int
	RegisterName string
	RiskLevel    string
	ColorCode    string
	Bucket       string
	Count        int
}

// RepeatedRiskRow is one repository row for the repeated-risks table before
// the service groups rows by title.
type RepeatedRiskRow struct {
	RiskTitle    string
	RegisterName string
	Status       string
	RiskLevel    string
	ColorCode    string
}

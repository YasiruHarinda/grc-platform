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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EntityClient is a thin wrapper over the compliance-entity REST API. Auth is
// handled by the Choreo gateway / Connection, so no token is set here.
// Attribution is by the createdBy / updatedBy fields in each JSON body (the
// entity trusts them — see internal/middleware UserIDToken), not by any header.
type EntityClient struct {
	baseURL string
	http    *http.Client
}

func NewEntityClient(baseURL string, timeout time.Duration) *EntityClient {
	return &EntityClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

// APIError is returned by every EntityClient call for a non-2xx response. It
// carries the status and the raw (trimmed) response body — the entity's error
// bodies are {"code":<int>,"message":<string>} (internal/apierror), but the
// body is kept verbatim so an unexpected shape is still legible in the log.
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// IsNotFound / IsValidation / IsConflict classify the status so callers can
// branch without matching on the number. 422 is folded into IsValidation
// defensively — the entity only emits 400 today (internal/handler/decode.go).
func (e *APIError) IsNotFound() bool { return e.Status == http.StatusNotFound }
func (e *APIError) IsValidation() bool {
	return e.Status == http.StatusBadRequest || e.Status == http.StatusUnprocessableEntity
}
func (e *APIError) IsConflict() bool { return e.Status == http.StatusConflict }

// AsAPIError pulls an *APIError out of err's chain, if one is there.
func AsAPIError(err error) (*APIError, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// do executes one JSON request. body is marshaled as the request body when
// non-nil; the response body is decoded into out when out is non-nil. Any
// non-2xx response becomes an *APIError; a transport failure is returned as-is.
func (e *EntityClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s %s: marshal request: %w", method, path, err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, e.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%s %s: build request: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			Method: method, Path: path, Status: resp.StatusCode,
			Body: strings.TrimSpace(string(respBody)),
		}
	}

	if out != nil && len(bytes.TrimSpace(respBody)) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("%s %s: decode response: %w", method, path, err)
		}
	}
	return nil
}

// RefData holds the reference-data lookups every row needs, loaded once in
// preflight (plan §7).
type RefData struct {
	TeamIDByKey        map[string]int // lower(name) and lower(code) -> risk_team.id
	TeamCodeByID       map[int]string // id -> code ("" when the team has none)
	CategoryIDByName   map[string]int // lower(name) -> risk_category.id
	ComplianceIDByName map[string]int // upper(name) -> risk_security_compliance_reference.id
	RoleIDByName       map[string]int // role_name -> role.id (the three risk roles)
}

// ── Request / response shapes (subset of the entity's domain we use) ─────────
//
// Every field tag below is verified against
// entity/compliance-entity/internal/domain (entity.go, grant.go). These are
// trimmed views: JSON decoding ignores the entity fields we don't list.

type CreateRiskRequest struct {
	RiskTitle              string            `json:"riskTitle"`
	RiskDescription        *string           `json:"riskDescription,omitempty"`
	SourceRegisterID       int               `json:"sourceRegisterId"`
	AssignmentTeamID       int               `json:"assignmentTeamId"`
	AssignerID             int               `json:"assignerId"`
	OwnerID                int               `json:"ownerId"`
	ManagementApproverID   int               `json:"managementApproverId"`
	RiskYear               int               `json:"riskYear"`
	RiskQuarter            string            `json:"riskQuarter"`
	Likelihood             int               `json:"likelihood"`
	Impact                 int               `json:"impact"`
	TreatmentStrategy      *string           `json:"treatmentStrategy,omitempty"`
	ImplementationDate     *string           `json:"implementationDate,omitempty"`
	ReassessmentDate       *string           `json:"reassessmentDate,omitempty"`
	ImpactDescription      *string           `json:"impactDescription,omitempty"`
	RiskIdentifiedDate     *string           `json:"riskIdentifiedDate,omitempty"`
	IdentifiedByType       *string           `json:"identifiedByType,omitempty"`
	IdentifiedByName       *string           `json:"identifiedByName,omitempty"`
	GitIssueURL            *string           `json:"gitIssueUrl,omitempty"`
	EmailSubject           *string           `json:"emailSubject,omitempty"`
	Remarks                *string           `json:"remarks,omitempty"`
	Progress               *string           `json:"progress,omitempty"`
	ActionOwnerID          *int              `json:"actionOwnerId,omitempty"`
	ActionPlanDescription  *string           `json:"actionPlanDescription,omitempty"`
	ActionSteps            []ActionStepInput `json:"actionSteps,omitempty"`
	ComplianceReferenceIDs []int             `json:"complianceReferenceIds,omitempty"`
	RiskCategoryIDs        []int             `json:"riskCategoryIds,omitempty"`
	CreatedBy              string            `json:"createdBy"`
}

type ActionStepInput struct {
	Description string `json:"description"`
}

// PatchRiskRequest is a subset of domain.UpdateRiskRequest. ExpectedStatus
// makes the PATCH a compare-and-set (a mismatch is a 409).
type PatchRiskRequest struct {
	WorkflowStatus         *string `json:"workflowStatus,omitempty"`
	ExpectedStatus         *string `json:"expectedStatus,omitempty"`
	ComplianceApprovalBy   *int    `json:"complianceApprovalBy,omitempty"`
	ComplianceApprovalDate *string `json:"complianceApprovalDate,omitempty"`
	UpdatedBy              string  `json:"updatedBy"`
}

// PatchActionPlanRequest is a subset of domain.UpdateRiskActionPlanRequest.
type PatchActionPlanRequest struct {
	Status        *string `json:"status,omitempty"`
	CompletedDate *string `json:"completedDate,omitempty"`
	UpdatedBy     string  `json:"updatedBy"`
}

// CreateEscalationRequest is a subset of domain.CreateRiskEscalationRequest —
// D8 sends only createdBy and omits every optional column.
type CreateEscalationRequest struct {
	CreatedBy string `json:"createdBy"`
}

type CreateUserRequest struct {
	UUID      string `json:"uuid"`
	UserType  string `json:"userType"` // INTERNAL
	Status    string `json:"status"`   // ACTIVE
	CreatedBy string `json:"createdBy"`
}

type CreateGrantRequest struct {
	RoleID    int    `json:"roleId"`
	ScopeType string `json:"scopeType"` // GLOBAL | RISK_TEAM
	ScopeID   int    `json:"scopeId"`   // 0 for GLOBAL
	CreatedBy string `json:"createdBy"`
}

// SearchRisksRequest is the subset of domain.SearchRisksRequest the resume
// query needs (plan §8 / T6). The entity has NO createdBy filter, so callers
// scope by register / year / quarter and filter the marker client-side.
type SearchRisksRequest struct {
	SourceRegisterIDs []int      `json:"sourceRegisterIds,omitempty"`
	RiskYears         []int      `json:"riskYears,omitempty"`
	RiskQuarterKeys   []string   `json:"riskQuarterKeys,omitempty"`
	Pagination        Pagination `json:"pagination"`
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// SearchRisksResponse mirrors domain.SearchRisksResponse.
type SearchRisksResponse struct {
	Risks  []Risk `json:"risks"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// Risk is the trimmed view we need back from POST /risks and /risks/search.
type Risk struct {
	ID             int    `json:"id"`
	RiskCode       string `json:"riskCode"`
	RiskTitle      string `json:"riskTitle"`
	RiskYear       int    `json:"riskYear"`
	RiskQuarter    string `json:"riskQuarter"`
	SourceRegID    int    `json:"sourceRegisterId"`
	WorkflowStatus string `json:"workflowStatus"`
	ActionPlanID   *int   `json:"actionPlanId"`
	CreatedBy      string `json:"createdBy"`
}

type Role struct {
	ID       int    `json:"id"`
	RoleName string `json:"roleName"`
	Module   string `json:"module"`
	Status   string `json:"status"`
}

// The three risk roles this migration grants (plan §6 / D10). Resolved to ids
// via GET /roles in buildRefData; all must exist and be ACTIVE.
const (
	roleRiskOwner      = "grc-platform-risk-owner"
	roleRiskAssigner   = "grc-platform-risk-assigner"
	roleRiskManagement = "grc-platform-risk-management"
)

// Trimmed reference-data views. Field tags verified against
// entity/compliance-entity/internal/domain/entity.go.

type RiskTeam struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Code   *string `json:"code"` // NULL for Legal / HR — cannot be a source register
	Status string  `json:"status"`
}

type RiskCategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ComplianceRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type RiskScore struct {
	ID         int `json:"id"`
	Likelihood int `json:"likelihood"`
	Impact     int `json:"impact"`
}

// refDataPageLimit is requested on the two paged reference-data searches. The
// tables are tiny (teams ~9, compliance refs ~6); a page this size fetches
// them whole, and the caller still guards on the response's Total.
const refDataPageLimit = 1000

// Escalation is the trimmed view from GET /risks/{riskId}/escalations. The
// entity types createdBy as *string; JSON null decodes to "" here, which is
// what the marker comparison in state.go wants.
type Escalation struct {
	ID        int    `json:"id"`
	Status    string `json:"status"` // OPEN | RESOLVED
	CreatedBy string `json:"createdBy"`
}

type Grant struct {
	RoleID    int    `json:"roleId"`
	ScopeType string `json:"scopeType"`
	ScopeID   int    `json:"scopeId"`
}

// ── Calls ──────────────────────────────────────────────────────────────────

// Health hits GET /health (routes.go — the plan's "/health-check" is wrong).
func (e *EntityClient) Health(ctx context.Context) error {
	return e.do(ctx, http.MethodGet, "/health", nil, nil)
}

// ListRoles unwraps GET /roles ({"roles":[...]} — domain.ListRolesResponse).
func (e *EntityClient) ListRoles(ctx context.Context) ([]Role, error) {
	var resp struct {
		Roles []Role `json:"roles"`
	}
	if err := e.do(ctx, http.MethodGet, "/roles", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Roles, nil
}

// ListRiskTeams returns the ACTIVE risk teams (POST /risk/teams/search).
func (e *EntityClient) ListRiskTeams(ctx context.Context) ([]RiskTeam, error) {
	body := struct {
		StatusKey  string     `json:"statusKey"`
		Pagination Pagination `json:"pagination"`
	}{StatusKey: "ACTIVE", Pagination: Pagination{Limit: refDataPageLimit}}

	var resp struct {
		Teams []RiskTeam `json:"teams"`
		Total int        `json:"total"`
	}
	if err := e.do(ctx, http.MethodPost, "/risk/teams/search", body, &resp); err != nil {
		return nil, err
	}
	if resp.Total > len(resp.Teams) {
		return nil, fmt.Errorf("risk_team has %d rows but only %d returned; raise refDataPageLimit", resp.Total, len(resp.Teams))
	}
	return resp.Teams, nil
}

// ListRiskCategories unwraps GET /risk/categories ({"categories":[...]}).
func (e *EntityClient) ListRiskCategories(ctx context.Context) ([]RiskCategory, error) {
	var resp struct {
		Categories []RiskCategory `json:"categories"`
	}
	if err := e.do(ctx, http.MethodGet, "/risk/categories", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Categories, nil
}

// ListComplianceRefs unwraps POST /risk/compliance-references/search
// ({"references":[...],"total":N}).
func (e *EntityClient) ListComplianceRefs(ctx context.Context) ([]ComplianceRef, error) {
	body := struct {
		Pagination Pagination `json:"pagination"`
	}{Pagination: Pagination{Limit: refDataPageLimit}}

	var resp struct {
		References []ComplianceRef `json:"references"`
		Total      int             `json:"total"`
	}
	if err := e.do(ctx, http.MethodPost, "/risk/compliance-references/search", body, &resp); err != nil {
		return nil, err
	}
	if resp.Total > len(resp.References) {
		return nil, fmt.Errorf("risk_security_compliance_reference has %d rows but only %d returned; raise refDataPageLimit", resp.Total, len(resp.References))
	}
	return resp.References, nil
}

// ListRiskScores unwraps GET /risk/scores ({"scores":[...]}).
func (e *EntityClient) ListRiskScores(ctx context.Context) ([]RiskScore, error) {
	var resp struct {
		Scores []RiskScore `json:"scores"`
	}
	if err := e.do(ctx, http.MethodGet, "/risk/scores", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Scores, nil
}

// ActionPlanView is the trimmed view of one row from
// GET /risks/{riskId}/action-plans.
type ActionPlanView struct {
	ID       int    `json:"id"`
	Status   string `json:"status"`   // PENDING | IN_PROGRESS | COMPLETED
	PlanType string `json:"planType"` // STANDARD | MANAGEMENT
}

// ListActionPlans unwraps GET /risks/{riskId}/action-plans ({"plans":[...]} —
// domain.ListRiskActionPlansResponse).
func (e *EntityClient) ListActionPlans(ctx context.Context, riskID int) ([]ActionPlanView, error) {
	var resp struct {
		Plans []ActionPlanView `json:"plans"`
	}
	if err := e.do(ctx, http.MethodGet, fmt.Sprintf("/risks/%d/action-plans", riskID), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Plans, nil
}

// buildRefData assembles the lookup maps every row needs and resolves the three
// risk role ids. It errors if any reference table is empty, if no team carries
// a code (a source register needs one for the risk_code), if two team keys
// collide on different ids, or if a risk role is missing or not ACTIVE.
func buildRefData(teams []RiskTeam, cats []RiskCategory, refs []ComplianceRef, scores []RiskScore, roles []Role) (RefData, error) {
	rd := RefData{
		TeamIDByKey:        map[string]int{},
		TeamCodeByID:       map[int]string{},
		CategoryIDByName:   map[string]int{},
		ComplianceIDByName: map[string]int{},
		RoleIDByName:       map[string]int{},
	}

	putTeamKey := func(key string, id int) error {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		if existing, ok := rd.TeamIDByKey[key]; ok && existing != id {
			return fmt.Errorf("risk_team key %q maps to both id %d and id %d", key, existing, id)
		}
		rd.TeamIDByKey[key] = id
		return nil
	}
	for _, t := range teams {
		code := ""
		if t.Code != nil {
			code = strings.TrimSpace(*t.Code)
		}
		rd.TeamCodeByID[t.ID] = code
		if err := putTeamKey(strings.ToLower(t.Name), t.ID); err != nil {
			return RefData{}, err
		}
		if err := putTeamKey(strings.ToLower(code), t.ID); err != nil {
			return RefData{}, err
		}
	}
	for _, c := range cats {
		rd.CategoryIDByName[strings.ToLower(strings.TrimSpace(c.Name))] = c.ID
	}
	for _, r := range refs {
		rd.ComplianceIDByName[strings.ToUpper(strings.TrimSpace(r.Name))] = r.ID
	}

	// scores itself isn't indexed into RefData — POST /risks resolves
	// gross_score_id server-side from (likelihood, impact) — but its presence
	// is still a preflight precondition (plan §7): an empty risk_score table
	// means every create would fail regardless of what the CSV says.
	if len(teams) == 0 || len(cats) == 0 || len(refs) == 0 || len(scores) == 0 {
		return RefData{}, fmt.Errorf(
			"reference data incomplete: teams=%d categories=%d complianceRefs=%d scores=%d (all must be non-empty)",
			len(teams), len(cats), len(refs), len(scores))
	}
	hasCode := false
	for _, code := range rd.TeamCodeByID {
		if code != "" {
			hasCode = true
			break
		}
	}
	if !hasCode {
		return RefData{}, fmt.Errorf("no risk_team carries a code; a source register needs one for the risk_code format")
	}

	byName := make(map[string]Role, len(roles))
	for _, r := range roles {
		byName[r.RoleName] = r
	}
	for _, want := range []string{roleRiskOwner, roleRiskAssigner, roleRiskManagement} {
		r, ok := byName[want]
		if !ok {
			return RefData{}, fmt.Errorf("risk role %q not found via GET /roles", want)
		}
		if !strings.EqualFold(r.Status, "ACTIVE") {
			return RefData{}, fmt.Errorf("risk role %q has status %q, want ACTIVE", want, r.Status)
		}
		rd.RoleIDByName[want] = r.ID
	}
	return rd, nil
}

// SearchRisks is a faithful wrapper over POST /risks/search. Resume-state logic
// (union of scopes, paging, marker filter) lives in state.go (plan §8 / T6).
func (e *EntityClient) SearchRisks(ctx context.Context, req SearchRisksRequest) (*SearchRisksResponse, error) {
	var resp SearchRisksResponse
	if err := e.do(ctx, http.MethodPost, "/risks/search", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateRisk posts to /risks (201; born PENDING_RISK_OWNER_APPROVAL) and
// returns the created risk incl. id and actionPlanId.
func (e *EntityClient) CreateRisk(ctx context.Context, req CreateRiskRequest) (*Risk, error) {
	var risk Risk
	if err := e.do(ctx, http.MethodPost, "/risks", req, &risk); err != nil {
		return nil, err
	}
	return &risk, nil
}

func (e *EntityClient) PatchRisk(ctx context.Context, riskID int, req PatchRiskRequest) (*Risk, error) {
	var risk Risk
	if err := e.do(ctx, http.MethodPatch, fmt.Sprintf("/risks/%d", riskID), req, &risk); err != nil {
		return nil, err
	}
	return &risk, nil
}

func (e *EntityClient) PatchActionPlan(ctx context.Context, planID int, req PatchActionPlanRequest) error {
	return e.do(ctx, http.MethodPatch, fmt.Sprintf("/action-plans/%d", planID), req, nil)
}

// ListEscalations unwraps GET /risks/{riskId}/escalations
// ({"escalations":[...]} — domain.ListRiskEscalationsResponse).
func (e *EntityClient) ListEscalations(ctx context.Context, riskID int) ([]Escalation, error) {
	var resp struct {
		Escalations []Escalation `json:"escalations"`
	}
	if err := e.do(ctx, http.MethodGet, fmt.Sprintf("/risks/%d/escalations", riskID), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Escalations, nil
}

func (e *EntityClient) CreateEscalation(ctx context.Context, riskID int, req CreateEscalationRequest) error {
	return e.do(ctx, http.MethodPost, fmt.Sprintf("/risks/%d/escalations", riskID), req, nil)
}

// GetUserByUUID returns (0, false, nil) on a 404 — the caller then provisions
// the user. Any other non-2xx is a real error.
func (e *EntityClient) GetUserByUUID(ctx context.Context, uuid string) (userID int, found bool, err error) {
	var u struct {
		ID int `json:"id"`
	}
	if err := e.do(ctx, http.MethodGet, "/users/by-uuid/"+url.PathEscape(uuid), nil, &u); err != nil {
		if ae, ok := AsAPIError(err); ok && ae.IsNotFound() {
			return 0, false, nil
		}
		return 0, false, err
	}
	return u.ID, true, nil
}

// CreateUser upserts on uuid (POST /users, 201) and returns the row id.
func (e *EntityClient) CreateUser(ctx context.Context, req CreateUserRequest) (userID int, err error) {
	var u struct {
		ID int `json:"id"`
	}
	if err := e.do(ctx, http.MethodPost, "/users", req, &u); err != nil {
		return 0, err
	}
	return u.ID, nil
}

// ListGrants unwraps GET /grants/user/{id}
// ({"userId":N,"grants":[...]} — domain.UserGrantsResponse).
func (e *EntityClient) ListGrants(ctx context.Context, userID int) ([]Grant, error) {
	var resp struct {
		Grants []Grant `json:"grants"`
	}
	if err := e.do(ctx, http.MethodGet, fmt.Sprintf("/grants/user/%d", userID), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Grants, nil
}

// CreateGrant posts to /grants/user/{id} (201). Idempotent server-side —
// re-granting reactivates rather than erroring on the unique index.
func (e *EntityClient) CreateGrant(ctx context.Context, userID int, req CreateGrantRequest) error {
	return e.do(ctx, http.MethodPost, fmt.Sprintf("/grants/user/%d", userID), req, nil)
}

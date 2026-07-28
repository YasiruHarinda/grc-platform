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
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/repository"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/entityclient"
)

type riskRepository struct{ c *entityclient.Client }

// NewRiskRepository creates a Compliance Entity-backed repository.RiskRepository.
func NewRiskRepository(c *entityclient.Client) repository.RiskRepository {
	return &riskRepository{c: c}
}

// entRisk is the entity's camelCase risk. Only the fields this package maps are
// listed; the entity returns more.
type entRisk struct {
	ID                     int       `json:"id"`
	RiskCode               string    `json:"riskCode"`
	RiskYear               int       `json:"riskYear"`
	RiskQuarter            string    `json:"riskQuarter"`
	RiskTitle              string    `json:"riskTitle"`
	RiskDescription        *string   `json:"riskDescription"`
	SourceRegisterID       int       `json:"sourceRegisterId"`
	SourceRegisterName     string    `json:"sourceRegisterName"`
	AssignmentTeamID       int       `json:"assignmentTeamId"`
	AssignmentTeamName     string    `json:"assignmentTeamName"`
	AssignerID             int       `json:"assignerId"`
	AssignerName           string    `json:"assignerName"`
	OwnerID                int       `json:"ownerId"`
	OwnerName              string    `json:"ownerName"`
	WorkflowStatus         string    `json:"workflowStatus"`
	TreatmentStrategy      *string   `json:"treatmentStrategy"`
	GrossScoreID           *int      `json:"grossScoreId"`
	ImplementationDate     *string   `json:"implementationDate"`
	ReassessmentDate       *string   `json:"reassessmentDate"`
	CreatedOn              time.Time `json:"createdOn"`
	UpdatedOn              time.Time `json:"updatedOn"`
	RiskIdentifiedDate     *string   `json:"riskIdentifiedDate"`
	IdentifiedByType       *string   `json:"identifiedByType"`
	IdentifiedByName       *string   `json:"identifiedByName"`
	ImpactDescription      *string   `json:"impactDescription"`
	ActionPlanID           *int      `json:"actionPlanId"`
	Progress               *string   `json:"progress"`
	ComplianceApprovalBy   *int      `json:"complianceApprovalBy"`
	ComplianceApprovalDate *string   `json:"complianceApprovalDate"`
	GitIssueURL            *string   `json:"gitIssueUrl"`
	EmailSubject           *string   `json:"emailSubject"`
	Remarks                *string   `json:"remarks"`
	RiskType               string    `json:"riskType"`
	RejectionComment       *string   `json:"rejectionComment"`
	RejectionStage         *string   `json:"rejectionStage"`
	OwnerFirstApprovedAt   *string   `json:"ownerFirstApprovedAt"`
	CreatedBy              string    `json:"createdBy"`
	UpdatedBy              string    `json:"updatedBy"`
	EffectiveRiskLevel     *string   `json:"effectiveRiskLevel"`
	EffectiveColorCode     *string   `json:"effectiveColorCode"`
}

type entScore struct {
	ID         int    `json:"id"`
	Likelihood int    `json:"likelihood"`
	Impact     int    `json:"impact"`
	RiskRating int    `json:"riskRating"`
	RiskLevel  string `json:"riskLevel"`
	ColorCode  string `json:"colorCode"`
}

func (s *entScore) toModel() *model.RiskScore {
	if s == nil {
		return nil
	}
	return &model.RiskScore{
		ID:         s.ID,
		Likelihood: s.Likelihood,
		Impact:     s.Impact,
		RiskRating: s.RiskRating,
		RiskLevel:  s.RiskLevel,
		ColorCode:  s.ColorCode,
	}
}

// deref returns the pointed-to string, or "" — the MySQL implementation
// COALESCEs these columns to empty strings rather than exposing null.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// List maps the backend's filter onto the entity's search request. Every filter
// has a direct counterpart; the entity resolves risk levels from the effective
// score, matching the MySQL query's effectiveScoreLeftJoin.
func (r *riskRepository) List(ctx context.Context, filter model.ListRisksFilter) (*model.RiskListPage, error) {
	body := map[string]any{
		"searchQuery":        filter.Search,
		"workflowStatusKeys": filter.Statuses,
		"sourceRegisterIds":  filter.TeamIDs,
		"riskLevelKeys":      filter.Levels,
		"riskTypeKeys":       filter.RiskTypes,
		"ownerIds":           filter.OwnerIDs,
		"submittedFrom":      filter.SubmittedFrom,
		"submittedTo":        filter.SubmittedTo,
		"dueFrom":            filter.DueFrom,
		"dueTo":              filter.DueTo,
		"dueOverdueOnly":     filter.DueOverdueOnly,
		"actionOwnerId":      filter.ActionOwnerID,
		"pagination":         map[string]int{"limit": filter.Limit, "offset": filter.Offset},
	}

	var resp struct {
		Risks []entRisk `json:"risks"`
		Total int       `json:"total"`
	}
	if err := r.c.Post(ctx, "/risks/search", body, &resp); err != nil {
		return nil, fmt.Errorf("list risks: %w", err)
	}

	page := &model.RiskListPage{
		Items:  make([]*model.RiskListItem, 0, len(resp.Risks)),
		Total:  resp.Total,
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}
	for _, e := range resp.Risks {
		page.Items = append(page.Items, &model.RiskListItem{
			ID:                 e.ID,
			RiskCode:           e.RiskCode,
			RiskTitle:          e.RiskTitle,
			SourceRegisterName: e.SourceRegisterName,
			// The list shows the effective level, not the gross one.
			RiskLevel:      deref(e.EffectiveRiskLevel),
			RiskLevelColor: deref(e.EffectiveColorCode),
			OwnerName:      e.OwnerName,
			AssignerName:   e.AssignerName,
			WorkflowStatus: e.WorkflowStatus,
			RiskType:       e.RiskType,
			// Dates render as RFC3339 to match what database/sql produced from
			// the DATE column; see dateOnlyToRFC3339.
			ImplementationDate: dateOnlyPtrToRFC3339(e.ImplementationDate),
			RejectionComment:   e.RejectionComment,
			RejectionStage:     e.RejectionStage,
			CreatedAt:          e.CreatedOn.UTC().Format(time.RFC3339Nano),
		})
	}
	return page, nil
}

// dateOnlyPtrToRFC3339 is dateOnlyToRFC3339 for optional dates, preserving nil.
func dateOnlyPtrToRFC3339(s *string) *string {
	if s == nil {
		return nil
	}
	v := dateOnlyToRFC3339(*s)
	return &v
}

// GetByID composes the risk detail from the entity's single detail endpoint,
// then appends the synthetic "initial" assessment representing the gross score.
// That entry is presentation, not data — it exists so the assessment log reads
// gross → reassessment → reassessment — so it is built here rather than stored.
func (r *riskRepository) GetByID(ctx context.Context, id int) (*model.RiskDetail, error) {
	var e struct {
		entRisk
		ComplianceApproverName *string   `json:"complianceApproverName"`
		GrossScore             *entScore `json:"grossScore"`
		EffectiveScore         *entScore `json:"effectiveScore"`
		ComplianceReferences   []struct {
			ID          int     `json:"id"`
			Name        string  `json:"name"`
			Description *string `json:"description"`
		} `json:"complianceReferences"`
		ActionPlan *struct {
			ID            int     `json:"id"`
			ActionOwnerID *int    `json:"actionOwnerId"`
			Description   *string `json:"description"`
			Status        string  `json:"status"`
			PlanType      string  `json:"planType"`
			Steps         []struct {
				ID            int     `json:"id"`
				PlanID        int     `json:"planId"`
				StepNo        int     `json:"stepNo"`
				Description   *string `json:"description"`
				Status        string  `json:"status"`
				CompletedDate *string `json:"completedDate"`
			} `json:"steps"`
		} `json:"actionPlan"`
		Assessments []entAssessment `json:"assessments"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/risks/%d/detail", id), &e); err != nil {
		return nil, fmt.Errorf("get risk %d: %w", id, err)
	}

	d := &model.RiskDetail{
		ID:                     e.ID,
		RiskCode:               e.RiskCode,
		RiskYear:               e.RiskYear,
		RiskQuarter:            e.RiskQuarter,
		RiskTitle:              e.RiskTitle,
		RiskDescription:        deref(e.RiskDescription),
		RiskIdentifiedDate:     dateOnlyPtrToRFC3339(e.RiskIdentifiedDate),
		IdentifiedByType:       e.IdentifiedByType,
		IdentifiedByName:       e.IdentifiedByName,
		AssignerID:             e.AssignerID,
		OwnerID:                e.OwnerID,
		ImpactDescription:      e.ImpactDescription,
		TreatmentStrategy:      e.TreatmentStrategy,
		AssignmentTeamID:       e.AssignmentTeamID,
		Progress:               e.Progress,
		ImplementationDate:     dateOnlyPtrToRFC3339(e.ImplementationDate),
		ReassessmentDate:       dateOnlyPtrToRFC3339(e.ReassessmentDate),
		GitIssueURL:            e.GitIssueURL,
		EmailSubject:           e.EmailSubject,
		Remarks:                e.Remarks,
		WorkflowStatus:         e.WorkflowStatus,
		RiskType:               e.RiskType,
		RejectionComment:       e.RejectionComment,
		RejectionStage:         e.RejectionStage,
		OwnerFirstApprovedAt:   dateOnlyPtrToRFC3339(e.OwnerFirstApprovedAt),
		ComplianceApprovalDate: dateOnlyPtrToRFC3339(e.ComplianceApprovalDate),
		CreatedAt:              e.CreatedOn.UTC().Format(time.RFC3339Nano),
		UpdatedAt:              e.UpdatedOn.UTC().Format(time.RFC3339Nano),
		SourceRegisterName:     e.SourceRegisterName,
		AssignmentTeamName:     e.AssignmentTeamName,
		OwnerName:              e.OwnerName,
		AssignerName:           e.AssignerName,
		ComplianceApproverName: e.ComplianceApproverName,
		GrossScore:             e.GrossScore.toModel(),
		EffectiveScore:         e.EffectiveScore.toModel(),
		ComplianceReferences:   []model.ComplianceReference{},
		Assessments:            []model.RiskAssessment{},
	}

	for _, ref := range e.ComplianceReferences {
		d.ComplianceReferences = append(d.ComplianceReferences, model.ComplianceReference{
			ID: ref.ID, Name: ref.Name, Description: ref.Description,
		})
	}

	if e.ActionPlan != nil {
		ap := model.ActionPlanDetail{
			ID:            e.ActionPlan.ID,
			ActionOwnerID: e.ActionPlan.ActionOwnerID,
			Description:   e.ActionPlan.Description,
			Status:        e.ActionPlan.Status,
			PlanType:      e.ActionPlan.PlanType,
			Steps:         []model.ActionPlanStep{},
		}
		for _, st := range e.ActionPlan.Steps {
			ap.Steps = append(ap.Steps, model.ActionPlanStep{
				ID:            st.ID,
				PlanID:        st.PlanID,
				StepNo:        st.StepNo,
				Description:   st.Description,
				Status:        st.Status,
				CompletedDate: dateOnlyPtrToRFC3339(st.CompletedDate),
			})
		}
		d.ActionPlan = &ap
	}

	for _, a := range e.Assessments {
		d.Assessments = append(d.Assessments, a.toModel())
	}

	// Synthetic oldest entry for the gross score, appended last because real
	// assessments arrive newest-first.
	if d.GrossScore != nil {
		baselineDate := d.CreatedAt
		if d.RiskIdentifiedDate != nil && *d.RiskIdentifiedDate != "" {
			baselineDate = *d.RiskIdentifiedDate
		}
		d.Assessments = append(d.Assessments, model.RiskAssessment{
			RiskID:             d.ID,
			ScoreID:            d.GrossScore.ID,
			ReassessmentDate:   baselineDate,
			ResidualLikelihood: d.GrossScore.Likelihood,
			ResidualImpact:     d.GrossScore.Impact,
			ResidualRating:     d.GrossScore.RiskRating,
			ResidualLevel:      d.GrossScore.RiskLevel,
			ResidualColorCode:  d.GrossScore.ColorCode,
			IsInitial:          true,
		})
	}

	return d, nil
}

// GetWorkflowStatus reads the summary risk rather than the detail composite —
// the whole point of this method is to guard a transition without paying for
// the related-entity queries.
func (r *riskRepository) GetWorkflowStatus(ctx context.Context, id int) (string, error) {
	var e entRisk
	if err := r.c.Get(ctx, fmt.Sprintf("/risks/%d", id), &e); err != nil {
		return "", fmt.Errorf("get workflow status for risk %d: %w", id, err)
	}
	return e.WorkflowStatus, nil
}

// Create sends the risk and everything created alongside it — action plan,
// steps, compliance links — as one request, so the entity commits them in a
// single transaction.
func (r *riskRepository) Create(ctx context.Context, req model.CreateRiskRequest, createdBy string) (*model.CreateRiskResponse, error) {
	steps := make([]map[string]any, 0, len(req.ActionSteps))
	for _, s := range req.ActionSteps {
		steps = append(steps, map[string]any{"description": s.Description})
	}

	body := map[string]any{
		"riskTitle":              req.RiskTitle,
		"riskDescription":        req.RiskDescription,
		"sourceRegisterId":       req.SourceRegisterID,
		"assignmentTeamId":       req.AssignmentTeamID,
		"assignerId":             req.AssignerID,
		"ownerId":                req.OwnerID,
		"riskYear":               req.Year,
		"riskQuarter":            req.Quarter,
		"likelihood":             req.Likelihood,
		"impact":                 req.Impact,
		"treatmentStrategy":      nullableString(req.TreatmentStrategy),
		"implementationDate":     nullableString(req.ImplementationDate),
		"reassessmentDate":       nullableString(req.ReassessmentDate),
		"impactDescription":      nullableString(req.ImpactDescription),
		"riskIdentifiedDate":     nullableString(req.RiskIdentifiedDate),
		"identifiedByType":       nullableString(req.IdentifiedByType),
		"identifiedByName":       req.IdentifiedByName,
		"gitIssueUrl":            nullableString(req.GitIssueURL),
		"emailSubject":           nullableString(req.EmailSubject),
		"remarks":                nullableString(req.Remarks),
		"progress":               nullableString(req.Progress),
		"actionOwnerId":          req.ActionOwnerID,
		"actionPlanDescription":  nullableString(req.ActionPlanDescription),
		"actionSteps":            steps,
		"complianceReferenceIds": req.ComplianceReferenceIDs,
		"createdBy":              createdBy,
	}

	var created entRisk
	if err := r.c.Post(ctx, "/risks", body, &created); err != nil {
		return nil, fmt.Errorf("create risk: %w", err)
	}
	return &model.CreateRiskResponse{ID: created.ID, RiskCode: created.RiskCode}, nil
}

// TransitionStatus moves the risk between two statuses, guarded by fromStatus
// so a concurrent change surfaces as 409 rather than silently overwriting.
func (r *riskRepository) TransitionStatus(ctx context.Context, id int, fromStatus, toStatus, updatedBy string) error {
	return r.patch(ctx, id, map[string]any{
		"workflowStatus": toStatus,
		"expectedStatus": fromStatus,
		"updatedBy":      updatedBy,
	}, "transition status")
}

// RejectTransition records the rejection and moves to PENDING_REVISION in one
// guarded write.
func (r *riskRepository) RejectTransition(ctx context.Context, id int, comment, stage, fromStatus, updatedBy string) error {
	return r.patch(ctx, id, map[string]any{
		"rejectionComment": comment,
		"rejectionStage":   stage,
		"workflowStatus":   model.StatusPendingRevision,
		"expectedStatus":   fromStatus,
		"updatedBy":        updatedBy,
	}, "reject transition")
}

// ResubmitTransition clears the rejection and advances the status.
//
// clearRejection rather than empty strings: the entity treats a nil field as
// "leave alone", so omitting them would keep the old rejection text on a
// resubmitted risk, and sending "" would store an empty string — which renders
// as a blank rejection banner instead of none. The MySQL statement sets both
// columns to NULL, and this is how that is expressed over JSON.
func (r *riskRepository) ResubmitTransition(ctx context.Context, id int, fromStatus, toStatus, updatedBy string) error {
	return r.patch(ctx, id, map[string]any{
		"clearRejection": true,
		"workflowStatus": toStatus,
		"expectedStatus": fromStatus,
		"updatedBy":      updatedBy,
	}, "resubmit transition")
}

// SetRiskType is unguarded, matching the MySQL implementation: it is called
// alongside a transition that already holds the guard.
func (r *riskRepository) SetRiskType(ctx context.Context, id int, riskType, updatedBy string) error {
	return r.patch(ctx, id, map[string]any{
		"riskType":  riskType,
		"updatedBy": updatedBy,
	}, "set risk type")
}

// SetOwnerFirstApprovedAt stamps the first owner approval. The MySQL version
// guards with `AND owner_first_approved_at IS NULL` so a later approval does not
// move the timestamp; the caller checks for nil before calling, so this only
// needs to write it.
func (r *riskRepository) SetOwnerFirstApprovedAt(ctx context.Context, id int, updatedBy string) error {
	return r.patch(ctx, id, map[string]any{
		"ownerFirstApprovedAt": time.Now().UTC().Format("2006-01-02"),
		"updatedBy":            updatedBy,
	}, "set owner first approved at")
}

// NextSequenceID previews the next risk code's sequence number without
// consuming it.
func (r *riskRepository) NextSequenceID(ctx context.Context, sourceRegisterID int) (int, error) {
	var resp struct {
		NextSequenceNumber int `json:"nextSequenceNumber"`
	}
	if err := r.c.Get(ctx, fmt.Sprintf("/risks/next-sequence-number?sourceRegisterId=%d", sourceRegisterID), &resp); err != nil {
		return 0, fmt.Errorf("next sequence id: %w", err)
	}
	return resp.NextSequenceNumber, nil
}

// patch issues a PATCH and discards the returned risk. Errors from the entity —
// including the 409 a failed compare-and-set produces — pass through unwrapped
// enough for the handler to map them to the right status.
func (r *riskRepository) patch(ctx context.Context, id int, body map[string]any, what string) error {
	if err := r.c.Patch(ctx, fmt.Sprintf("/risks/%d", id), body, nil); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// nullableString mirrors the MySQL package's helper: an empty string means the
// caller did not supply a value, and must serialise as JSON null rather than "".
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

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

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/repository"
)

type riskService struct{ repo repository.RiskRepository }

// NewRiskService constructs a RiskService.
func NewRiskService(repo repository.RiskRepository) RiskService {
	return &riskService{repo: repo}
}

// validRiskStatuses mirrors the risk.workflow_status ENUM in risk_schema.sql
// exactly (13 statuses). Keep in sync with the schema.
var validRiskStatuses = map[string]bool{
	"DRAFT":                               true,
	"PENDING_RISK_OWNER_APPROVAL":         true,
	"PENDING_MANAGEMENT_APPROVAL":         true,
	"PENDING_COMPLIANCE_REVIEW":           true,
	"IN_REMEDIATION":                      true,
	"PENDING_OWNER_COMPLETION_APPROVAL":   true,
	"PENDING_MANAGEMENT_CLOSURE_APPROVAL": true,
	"PENDING_COMPLIANCE_CLOSURE":          true,
	"PENDING_AMENDMENT":                   true,
	"PENDING_REVISION":                    true,
	"ESCALATED":                           true,
	"CLOSED":                              true,
	"CANCELLED":                           true,
}

var validRiskQuarters = map[string]bool{"Q1": true, "Q2": true, "Q3": true, "Q4": true}

// allowedRiskTransitions defines the legal next statuses for each risk
// workflow_status. It is derived from the GRC backend's risk service, which
// owns the workflow and is the authoritative definition of it:
//
//   - OwnerApprove: PENDING_RISK_OWNER_APPROVAL or PENDING_AMENDMENT →
//     PENDING_COMPLIANCE_REVIEW, or → PENDING_MANAGEMENT_APPROVAL when the
//     treatment is ACCEPT and the gross level is HIGH; and
//     PENDING_OWNER_COMPLETION_APPROVAL → PENDING_COMPLIANCE_CLOSURE
//   - ManagementApprove: PENDING_MANAGEMENT_APPROVAL → PENDING_COMPLIANCE_REVIEW
//   - Approve:          PENDING_COMPLIANCE_REVIEW → IN_REMEDIATION
//   - Reject:           any of the five pending-approval states → PENDING_REVISION
//   - Complete:         IN_REMEDIATION → PENDING_OWNER_COMPLETION_APPROVAL
//   - Resubmit:         PENDING_REVISION → PENDING_RISK_OWNER_APPROVAL, or →
//     PENDING_OWNER_COMPLETION_APPROVAL when the rejection stage was COMPLETION_OWNER
//   - Close:            PENDING_COMPLIANCE_CLOSURE → CLOSED
//   - Cancel:           PENDING_RISK_OWNER_APPROVAL → CANCELLED
//
// An earlier version of this map encoded a different workflow, written before
// the risk module existed, and would have rejected the backend's most common
// transition (PENDING_RISK_OWNER_APPROVAL → PENDING_COMPLIANCE_REVIEW) along
// with every rejection path. When the two disagree, the backend is right.
//
// This map is deliberately a superset: DRAFT and ESCALATED transitions are kept
// even though the backend never performs them, and CANCELLED is permitted from
// every non-terminal state. Being more permissive than the caller is safe —
// the backend decides which transition to make, and this only stops a move that
// is nonsense in every workflow. Being less permissive is not: it turns a
// legitimate action into a 409. CLOSED and CANCELLED remain terminal.
var allowedRiskTransitions = map[string][]string{
	"DRAFT": {"PENDING_RISK_OWNER_APPROVAL", "CANCELLED"},
	"PENDING_RISK_OWNER_APPROVAL": {
		"PENDING_COMPLIANCE_REVIEW", "PENDING_MANAGEMENT_APPROVAL",
		"PENDING_AMENDMENT", "PENDING_REVISION", "ESCALATED", "CANCELLED",
	},
	"PENDING_AMENDMENT": {
		"PENDING_COMPLIANCE_REVIEW", "PENDING_MANAGEMENT_APPROVAL",
		"PENDING_RISK_OWNER_APPROVAL", "PENDING_REVISION", "CANCELLED",
	},
	"PENDING_MANAGEMENT_APPROVAL": {
		"PENDING_COMPLIANCE_REVIEW", "PENDING_AMENDMENT",
		"PENDING_REVISION", "ESCALATED", "CANCELLED",
	},
	"PENDING_COMPLIANCE_REVIEW": {
		"IN_REMEDIATION", "PENDING_REVISION", "ESCALATED", "CANCELLED",
	},
	// PENDING_AMENDMENT: editing a restricted field (implementation_date or the
	// action steps) on an in-remediation risk sends it back through the approval
	// chain. Its absence here rejected a legitimate edit with a 400 — exactly
	// the "less permissive than the caller" failure this map's comment warns
	// about.
	"IN_REMEDIATION": {"PENDING_OWNER_COMPLETION_APPROVAL", "PENDING_AMENDMENT", "ESCALATED"},
	"PENDING_OWNER_COMPLETION_APPROVAL": {
		"PENDING_COMPLIANCE_CLOSURE", "PENDING_MANAGEMENT_CLOSURE_APPROVAL",
		"PENDING_REVISION", "IN_REMEDIATION",
	},
	// ACCEPT + HIGH only: the risk's management_approver_id signs off on the
	// completed remediation before compliance closes it.
	"PENDING_MANAGEMENT_CLOSURE_APPROVAL": {
		"PENDING_COMPLIANCE_CLOSURE", "PENDING_REVISION", "IN_REMEDIATION",
	},
	"PENDING_COMPLIANCE_CLOSURE": {"CLOSED", "IN_REMEDIATION"},
	"PENDING_REVISION": {
		"PENDING_RISK_OWNER_APPROVAL", "PENDING_OWNER_COMPLETION_APPROVAL",
		"PENDING_MANAGEMENT_CLOSURE_APPROVAL",
		"PENDING_COMPLIANCE_REVIEW", "CANCELLED",
	},
	// IN_REMEDIATION: the daily escalation job (internal/job) reverts a risk
	// here once its linked MANAGEMENT action plan completes — see
	// internal/service/risk_action_plan_service.go's completion cascade.
	"ESCALATED": {"PENDING_MANAGEMENT_APPROVAL", "IN_REMEDIATION", "CANCELLED"},
	"CLOSED":    {},
	"CANCELLED": {},
}

// isValidRiskTransition reports whether moving from → to is a legal workflow step.
// A no-op (from == to) and an empty current status are always allowed.
func isValidRiskTransition(from, to string) bool {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	if from == to || from == "" {
		return true
	}
	for _, next := range allowedRiskTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// validTreatmentStrategies / validIdentifiedByTypes mirror the risk.treatment_strategy
// and risk.identified_by_type ENUMs in risk_schema.sql. Both columns are nullable,
// so these are only enforced when a value is provided.
// REMEDIATE, not MITIGATE: the risk.treatment_strategy ENUM is
// ('REMEDIATE','ACCEPT','TRANSFER','VOID'). MITIGATE was accepted here and then
// rejected by MySQL as a truncated value, while every real REMEDIATE row was
// refused — the validator had it exactly backwards.
var validTreatmentStrategies = map[string]bool{"REMEDIATE": true, "ACCEPT": true, "TRANSFER": true, "VOID": true}
var validIdentifiedByTypes = map[string]bool{"EMPLOYEE": true, "EXTERNAL_PERSON": true, "TOOL": true}

// validRiskLevels mirrors the distinct risk_score.risk_level values;
// validRiskTypes mirrors the risk.risk_type ENUM.
var validRiskLevels = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true}
var validRiskTypes = map[string]bool{"NEW": true, "UPDATED": true}

func (s *riskService) SearchRisks(ctx context.Context, req domain.SearchRisksRequest) (domain.SearchRisksResponse, error) {
	for i, sk := range req.WorkflowStatusKeys {
		if !validRiskStatuses[strings.ToUpper(sk)] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid workflowStatusKey: " + sk}
		}
		req.WorkflowStatusKeys[i] = strings.ToUpper(sk)
	}
	for i, qk := range req.RiskQuarterKeys {
		if !validRiskQuarters[strings.ToUpper(qk)] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid riskQuarterKey: " + qk + " (must be Q1, Q2, Q3, or Q4)"}
		}
		req.RiskQuarterKeys[i] = strings.ToUpper(qk)
	}
	for i, lk := range req.RiskLevelKeys {
		up := strings.ToUpper(lk)
		if !validRiskLevels[up] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid riskLevelKey: " + lk + " (must be LOW, MEDIUM, or HIGH)"}
		}
		req.RiskLevelKeys[i] = up
	}
	for i, tk := range req.RiskTypeKeys {
		up := strings.ToUpper(tk)
		if !validRiskTypes[up] {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: "invalid riskTypeKey: " + tk + " (must be NEW or UPDATED)"}
		}
		req.RiskTypeKeys[i] = up
	}
	for _, d := range []struct{ name, value string }{
		{"submittedFrom", req.SubmittedFrom}, {"submittedTo", req.SubmittedTo},
		{"dueFrom", req.DueFrom}, {"dueTo", req.DueTo},
	} {
		if d.value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d.value); err != nil {
			return domain.SearchRisksResponse{}, &apierror.ValidationError{Msg: d.name + " must be a date in YYYY-MM-DD format"}
		}
	}

	normalizePagination(&req.Pagination)
	risks, total, err := s.repo.SearchRisks(ctx, req)
	if err != nil {
		return domain.SearchRisksResponse{}, err
	}
	if risks == nil {
		risks = []domain.Risk{}
	}
	return domain.SearchRisksResponse{Risks: risks, Total: total, Limit: req.Pagination.Limit, Offset: req.Pagination.Offset}, nil
}

func (s *riskService) GetRiskByID(ctx context.Context, id int) (domain.Risk, error) {
	if id <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "risk id must be a positive integer"}
	}
	r, err := s.repo.GetRiskByID(ctx, id)
	if err != nil {
		return domain.Risk{}, err
	}
	return *r, nil
}

func (s *riskService) CreateRisk(ctx context.Context, req domain.CreateRiskRequest) (domain.Risk, error) {
	if req.RiskTitle == "" {
		return domain.Risk{}, &apierror.ValidationError{Msg: "riskTitle is required"}
	}
	if req.SourceRegisterID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "sourceRegisterId is required"}
	}
	if req.AssignmentTeamID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "assignmentTeamId is required"}
	}
	if req.AssignerID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "assignerId is required"}
	}
	if req.OwnerID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "ownerId is required"}
	}
	if req.ManagementApproverID <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "managementApproverId is required"}
	}
	if req.RiskYear <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "riskYear is required"}
	}
	req.RiskQuarter = strings.ToUpper(req.RiskQuarter)
	if !validRiskQuarters[req.RiskQuarter] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "riskQuarter must be Q1, Q2, Q3, or Q4"}
	}
	// Bounds match the 3×3 risk_score matrix; the repository resolves the pair
	// to a score row and fails the create if no cell matches.
	if req.Likelihood < 1 || req.Likelihood > 3 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "likelihood must be 1, 2, or 3"}
	}
	if req.Impact < 1 || req.Impact > 3 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "impact must be 1, 2, or 3"}
	}
	if req.TreatmentStrategy != nil && !validTreatmentStrategies[strings.ToUpper(*req.TreatmentStrategy)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "treatmentStrategy must be REMEDIATE, ACCEPT, TRANSFER, or VOID"}
	}
	if req.TreatmentStrategy != nil {
		up := strings.ToUpper(*req.TreatmentStrategy)
		req.TreatmentStrategy = &up
	}
	if req.IdentifiedByType != nil && !validIdentifiedByTypes[strings.ToUpper(*req.IdentifiedByType)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "identifiedByType must be EMPLOYEE, EXTERNAL_PERSON, or TOOL"}
	}
	if req.IdentifiedByType != nil {
		up := strings.ToUpper(*req.IdentifiedByType)
		req.IdentifiedByType = &up
	}
	if req.CreatedBy == "" {
		return domain.Risk{}, &apierror.ValidationError{Msg: "createdBy is required"}
	}
	// Every risk must carry at least one non-empty action step. The backend
	// enforces this too, but it's re-checked here (the last writer before the
	// DB) so a direct caller can't create a stepless plan — which could never
	// be marked COMPLETED, permanently blocking the risk from being submitted
	// for completion approval.
	if len(req.ActionSteps) == 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "at least one action step is required"}
	}
	for i, step := range req.ActionSteps {
		if strings.TrimSpace(step.Description) == "" {
			return domain.Risk{}, &apierror.ValidationError{Msg: fmt.Sprintf("action step %d description is required", i+1)}
		}
	}
	r, err := s.repo.CreateRisk(ctx, req)
	if err != nil {
		return domain.Risk{}, err
	}
	return *r, nil
}

func (s *riskService) UpdateRisk(ctx context.Context, id int, req domain.UpdateRiskRequest) (domain.Risk, error) {
	if id <= 0 {
		return domain.Risk{}, &apierror.ValidationError{Msg: "risk id must be a positive integer"}
	}
	if req.UpdatedBy == "" {
		return domain.Risk{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if req.WorkflowStatus != nil && !validRiskStatuses[strings.ToUpper(*req.WorkflowStatus)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "invalid workflowStatus: " + *req.WorkflowStatus}
	}
	if req.WorkflowStatus != nil {
		up := strings.ToUpper(*req.WorkflowStatus)
		req.WorkflowStatus = &up
	}
	if req.WorkflowStatus != nil {
		// A caller that supplied expectedStatus already read the risk and made a
		// decision against that value — validate and guard on it, not on a fresh
		// read. Re-reading here would check a transition the caller never
		// decided on, and would let a concurrent change slip past the CAS.
		from := req.ExpectedStatus
		if from == "" {
			current, err := s.repo.GetRiskByID(ctx, id)
			if err != nil {
				return domain.Risk{}, err
			}
			from = current.WorkflowStatus
			req.ExpectedStatus = from
		}
		if !isValidRiskTransition(from, *req.WorkflowStatus) {
			return domain.Risk{}, &apierror.ValidationError{
				Msg: "invalid workflow transition: " + from + " → " + *req.WorkflowStatus,
			}
		}
	}
	if req.TreatmentStrategy != nil && !validTreatmentStrategies[strings.ToUpper(*req.TreatmentStrategy)] {
		return domain.Risk{}, &apierror.ValidationError{Msg: "treatmentStrategy must be REMEDIATE, ACCEPT, TRANSFER, or VOID"}
	}
	if req.TreatmentStrategy != nil {
		up := strings.ToUpper(*req.TreatmentStrategy)
		req.TreatmentStrategy = &up
	}
	r, err := s.repo.UpdateRisk(ctx, id, req)
	if err != nil {
		return domain.Risk{}, err
	}
	return *r, nil
}

// NextSequenceNumber previews the sequence number the next risk created for
// this source register would get. It consumes nothing — CreateRisk owns the
// increment.
func (s *riskService) NextSequenceNumber(ctx context.Context, sourceRegisterID int) (domain.NextSequenceResponse, error) {
	if sourceRegisterID <= 0 {
		return domain.NextSequenceResponse{}, &apierror.ValidationError{Msg: "sourceRegisterId must be a positive integer"}
	}
	n, err := s.repo.NextSequenceNumber(ctx, sourceRegisterID)
	if err != nil {
		return domain.NextSequenceResponse{}, err
	}
	return domain.NextSequenceResponse{NextSequenceNumber: n}, nil
}

// GetRiskDetail returns the fully-composed risk: every column, resolved names,
// both scores, and the related references, action plan, steps and assessments.
func (s *riskService) GetRiskDetail(ctx context.Context, id int) (domain.RiskDetail, error) {
	if id <= 0 {
		return domain.RiskDetail{}, &apierror.ValidationError{Msg: "risk id must be a positive integer"}
	}
	d, err := s.repo.GetRiskDetail(ctx, id)
	if err != nil {
		return domain.RiskDetail{}, err
	}
	return *d, nil
}

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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// Update edits a risk and everything attached to it.
//
// The workflow decisions stay here, in the module that owns them: which fields
// are restricted, whether an edit forces re-approval, what belongs in the change
// log, and which fields stop being editable once an owner has approved. The
// entity is told what to write and makes it atomic.
//
// The read below happens outside the write, exactly as the MySQL implementation
// reads the risk before opening its transaction. The window that opens is closed
// the same way: the write carries expectedStatus, so a status that moved in
// between produces a 409 instead of an edit applied against stale assumptions.
func (r *riskRepository) Update(ctx context.Context, id int, req model.UpdateRiskRequest, updatedBy string) error {
	current, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if current.WorkflowStatus == model.StatusClosed {
		return &apierror.Error{
			StatusCode: http.StatusConflict,
			Body:       "risk is closed and can no longer be edited",
		}
	}

	// Gross score and reassessment date are full-edit-only: once a risk owner
	// has approved, they are fixed and any incoming value is discarded rather
	// than rejected, so an otherwise valid edit still goes through.
	if current.OwnerFirstApprovedAt != nil && *current.OwnerFirstApprovedAt != "" {
		req.GrossScoreID = nil
		req.ReassessmentDate = ""
	}

	// Restricted fields force re-approval when changed on an IN_REMEDIATION
	// risk. Only a non-empty incoming value counts as a change — an omitted
	// field is not an instruction to clear it.
	var changeLog []map[string]any
	restrictedChanged := false
	noteChange := func(field, oldVal, newVal string) {
		if newVal == "" || oldVal == newVal {
			return
		}
		restrictedChanged = true
		oldJSON, _ := json.Marshal(oldVal)
		newJSON, _ := json.Marshal(newVal)
		changeLog = append(changeLog, map[string]any{
			"action":       "UPDATE",
			"fieldChanged": field,
			"oldValue":     string(oldJSON),
			"newValue":     string(newJSON),
		})
	}
	noteChange("implementation_date", derefOr(current.ImplementationDate), req.ImplementationDate)
	noteChange("email_subject", derefOr(current.EmailSubject), req.EmailSubject)

	stepsChanged := actionStepsChanged(current, req)
	if stepsChanged {
		restrictedChanged = true
		changeLog = append(changeLog, map[string]any{
			"action":       "UPDATE",
			"fieldChanged": "action_steps",
		})
	}

	body := map[string]any{
		"riskTitle":       req.RiskTitle,
		"riskDescription": req.RiskDescription,
		"updatedBy":       updatedBy,
	}
	// Only send fields the caller actually supplied: the entity overwrites any
	// field present in the payload, so sending "" would blank the column.
	putIfSet := func(key, value string) {
		if value != "" {
			body[key] = value
		}
	}
	putIfSet("riskIdentifiedDate", req.RiskIdentifiedDate)
	putIfSet("identifiedByType", req.IdentifiedByType)
	putIfSet("impactDescription", req.ImpactDescription)
	putIfSet("progress", req.Progress)
	putIfSet("gitIssueUrl", req.GitIssueURL)
	putIfSet("remarks", req.Remarks)
	putIfSet("implementationDate", req.ImplementationDate)
	putIfSet("reassessmentDate", req.ReassessmentDate)
	putIfSet("treatmentStrategy", req.TreatmentStrategy)
	if req.IdentifiedByName != nil {
		body["identifiedByName"] = *req.IdentifiedByName
	}
	if req.AssignerID != nil {
		body["assignerId"] = *req.AssignerID
	}
	if req.OwnerID != nil {
		body["ownerId"] = *req.OwnerID
	}
	if req.AssignmentTeamID != nil {
		body["assignmentTeamId"] = *req.AssignmentTeamID
	}
	if req.GrossScoreID != nil {
		body["grossScoreId"] = *req.GrossScoreID
	}
	// email_subject is always sent: the MySQL statement assigns it
	// unconditionally, so clearing it is a legitimate edit.
	body["emailSubject"] = req.EmailSubject

	if req.ComplianceReferenceIDs != nil {
		body["complianceReferenceIds"] = req.ComplianceReferenceIDs
	}
	if req.ActionPlanDescription != "" || req.ActionOwnerID != nil {
		plan := map[string]any{}
		if req.ActionPlanDescription != "" {
			plan["description"] = req.ActionPlanDescription
		}
		if req.ActionOwnerID != nil {
			plan["actionOwnerId"] = *req.ActionOwnerID
		}
		body["actionPlan"] = plan
	}
	if stepsChanged {
		steps := make([]map[string]any, 0, len(req.ActionSteps))
		for _, s := range req.ActionSteps {
			step := map[string]any{"description": s.Description}
			if s.ID != nil {
				step["id"] = *s.ID
			}
			steps = append(steps, step)
		}
		body["actionSteps"] = steps
	}
	if len(changeLog) > 0 {
		body["changeLog"] = changeLog
	}

	// A restricted change to a risk under remediation sends it back for
	// amendment, and marks it as an update rather than a new risk. Guarded on
	// the status read above so a concurrent transition rolls the edit back.
	if restrictedChanged && current.WorkflowStatus == model.StatusInRemediation {
		body["riskType"] = model.RiskTypeUpdated
		body["workflowStatus"] = model.StatusPendingAmendment
		body["expectedStatus"] = model.StatusInRemediation
	} else {
		body["expectedStatus"] = current.WorkflowStatus
	}

	if err := r.c.Patch(ctx, fmt.Sprintf("/risks/%d", id), body, nil); err != nil {
		return fmt.Errorf("update risk %d: %w", id, err)
	}
	return nil
}

// actionStepsChanged reports whether the incoming steps differ from what the
// plan holds. An empty payload means the caller is not editing steps at all —
// distinct from an explicit instruction to remove them.
//
// A missing plan, or a different number of steps, counts as changed without
// further comparison. Otherwise steps are compared pairwise in order: a step
// whose ID or description differs from the one in the same position is a change.
func actionStepsChanged(current *model.RiskDetail, req model.UpdateRiskRequest) bool {
	if len(req.ActionSteps) == 0 {
		return false
	}
	if current.ActionPlan == nil {
		return true
	}
	if len(current.ActionPlan.Steps) != len(req.ActionSteps) {
		return true
	}
	for i, want := range req.ActionSteps {
		have := current.ActionPlan.Steps[i]
		if want.ID == nil || *want.ID != have.ID || derefOr(have.Description) != want.Description {
			return true
		}
	}
	return false
}

// derefOr returns the pointed-to string, or "".
func derefOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

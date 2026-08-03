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

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/apierror"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/response"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/auth"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// handleNextSequenceID serves GET /api/v1/risks/next-sequence-id.
// Required query params: source_register_id, year, quarter.
// Returns a preview of the next available sequence number for the risk code.
// This does not reserve the number — the actual code is assigned atomically on POST.
func (d *Deps) handleNextSequenceID(w http.ResponseWriter, r *http.Request) {
	if !auth.RequirePrivilege(r.Context(), w, privilege.CreateRisk) {
		return
	}
	q := r.URL.Query()

	sourceRegisterIDStr := q.Get("source_register_id")
	yearStr := q.Get("year")
	quarter := q.Get("quarter")

	if sourceRegisterIDStr == "" || yearStr == "" || quarter == "" {
		response.WriteError(w, http.StatusBadRequest, "source_register_id, year, and quarter are required")
		return
	}

	sourceRegisterID, err := strconv.Atoi(sourceRegisterIDStr)
	if err != nil || sourceRegisterID <= 0 {
		response.WriteError(w, http.StatusBadRequest, "source_register_id must be a positive integer")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 {
		response.WriteError(w, http.StatusBadRequest, "year must be a valid 4-digit year")
		return
	}

	nextID, err := d.Risk.NextSequenceID(r.Context(), sourceRegisterID)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	response.WriteJSONValue(w, http.StatusOK, model.NextSequenceIDResponse{NextSequenceID: nextID})
}

// handleCreateRisk serves POST /api/v1/risks.
// Atomically generates a risk code, creates the risk, action plan, steps,
// compliance references, and change log entry inside a single DB transaction.
func (d *Deps) handleCreateRisk(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		response.WriteError(w, http.StatusUnauthorized, response.ErrMsgUnauthorized)
		return
	}
	if !auth.RequirePrivilege(r.Context(), w, privilege.CreateRisk) {
		return
	}

	var req model.CreateRiskRequest
	if err := response.DecodeJSON(w, r, &req); err != nil {
		return
	}

	if err := validateCreateRiskRequest(req); err != nil {
		response.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.IdentifiedByType == model.IdentifiedByEmployee {
		name, err := d.resolveIdentifiedByEmployee(r.Context(), *req.IdentifiedByEmail)
		if err != nil {
			response.MapServiceError(r.Context(), w, err, "Unable to verify the identifying employee. Please try again.")
			return
		}
		req.IdentifiedByName = &name
	} else if req.IdentifiedByType == model.IdentifiedByExternalPerson || req.IdentifiedByType == model.IdentifiedByTool {
		trimmed := strings.TrimSpace(*req.IdentifiedByName)
		req.IdentifiedByName = &trimmed
	}

	createdBy := user.Email
	if createdBy == "" {
		createdBy = user.Subject
	}
	result, err := d.Risk.Create(r.Context(), req, createdBy)
	if err != nil {
		response.MapServiceError(r.Context(), w, err, response.ErrMsgInternal)
		return
	}

	d.notifyRiskOwnerCreated(result.ID, req.OwnerID, createdBy)

	w.Header().Set("Location", fmt.Sprintf("/api/v1/risks/%d", result.ID))
	response.WriteJSONValue(w, http.StatusCreated, result)
}

// notifyTimeout caps the whole background notification: two Compliance Entity
// lookups plus an email send that retries once. It has to outlast that retry —
// sized too tightly, it would cancel the second attempt before it could land,
// which is the exact failure the retry exists to fix. The individual HTTP
// clients all have their own timeouts, so this is a backstop against a stuck
// goroutine rather than the mechanism that bounds any single call.
const notifyTimeout = 2 * time.Minute

// notifyRiskOwnerCreated emails the risk owner that a new risk has been
// assigned to them. Best-effort and detached: the risk is already committed by
// the time this runs, so any failure here is logged and swallowed rather than
// affecting the 201 response. It runs in the background on a context of its
// own — email-service cold starts take tens of seconds and the request context
// is cancelled the moment the handler returns, so doing this inline would both
// stall the response and cut the send off midway.
func (d *Deps) notifyRiskOwnerCreated(riskID, ownerID int, createdBy string) {
	go func() {
		// net/http recovers a panic raised on the request path; a bare
		// goroutine has no such net, so an unguarded panic here would take the
		// whole process down instead of failing one notification.
		defer func() {
			if p := recover(); p != nil {
				slog.Error("risk creation email: panic", "riskId", riskID, "panic", p)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		d.sendRiskOwnerCreatedEmail(ctx, riskID, ownerID, createdBy)
	}()
}

// sendRiskOwnerCreatedEmail resolves the owner and the risk detail, then sends
// the notification. Split out from notifyRiskOwnerCreated so the work is
// callable synchronously in a test without the goroutine.
func (d *Deps) sendRiskOwnerCreatedEmail(ctx context.Context, riskID, ownerID int, createdBy string) {
	owner, err := d.Users.GetByID(ctx, ownerID)
	if err != nil {
		slog.Warn("risk creation email: failed to resolve owner", "riskId", riskID, "ownerId", ownerID, "err", err)
		return
	}
	if owner == nil || owner.Email == "" {
		slog.Warn("risk creation email: owner has no email on file", "riskId", riskID, "ownerId", ownerID)
		return
	}

	detail, err := d.Risk.GetByID(ctx, riskID)
	if err != nil {
		slog.Warn("risk creation email: failed to load risk detail", "riskId", riskID, "err", err)
		return
	}

	riskLevel := ""
	if detail.GrossScore != nil {
		riskLevel = detail.GrossScore.RiskLevel
	}

	if err := d.Email.SendRiskCreated(ctx, owner.Email, emailer.RiskCreated{
		RiskCode:       detail.RiskCode,
		RiskTitle:      detail.RiskTitle,
		SourceRegister: detail.SourceRegisterName,
		RiskLevel:      riskLevel,
		CreatedBy:      createdBy,
		DetailURL:      fmt.Sprintf("%s/risk/registers?riskId=%d", d.FrontendBaseURL, riskID),
	}); err != nil {
		slog.Warn("risk creation email: send failed", "riskId", riskID, "ownerEmail", owner.Email, "err", err)
		return
	}
	slog.Info("risk creation email sent", "riskId", riskID, "ownerEmail", owner.Email)
}

// validateCreateRiskRequest performs business validation on the incoming payload.
func validateCreateRiskRequest(req model.CreateRiskRequest) error {
	if req.RiskTitle == "" {
		return errorf("risk_title is required")
	}
	if req.RiskDescription == "" {
		return errorf("risk_description is required")
	}
	if req.RiskIdentifiedDate == "" {
		return errorf("risk_identified_date is required")
	}
	if req.ImpactDescription == "" {
		return errorf("impact_description is required")
	}
	if req.ImplementationDate == "" {
		return errorf("implementation_date is required")
	}
	if req.ReassessmentDate == "" {
		return errorf("reassessment_date is required")
	}
	if req.SourceRegisterID <= 0 {
		return errorf("source_register_id is required")
	}
	if req.AssignmentTeamID <= 0 {
		return errorf("assignment_team_id is required")
	}
	if req.AssignerID <= 0 {
		return errorf("assigner_id is required")
	}
	if req.OwnerID <= 0 {
		return errorf("owner_id is required")
	}
	if req.ManagementApproverID <= 0 {
		return errorf("management_approver_id is required")
	}
	if req.ActionOwnerID <= 0 {
		return errorf("action_owner_id is required")
	}
	if req.Year < 2000 || req.Year > 2100 {
		return errorf("year must be a 4-digit year between 2000 and 2100")
	}
	switch req.Quarter {
	case "Q1", "Q2", "Q3", "Q4":
	default:
		return errorf("quarter must be Q1, Q2, Q3, or Q4")
	}
	if req.Likelihood < 1 || req.Likelihood > 3 {
		return errorf("likelihood must be 1, 2, or 3")
	}
	if req.Impact < 1 || req.Impact > 3 {
		return errorf("impact must be 1, 2, or 3")
	}
	if req.EmailSubject == "" {
		return errorf("email_subject is required")
	}
	if req.TreatmentStrategy == "" {
		return errorf("treatment_strategy is required")
	}
	if len(req.ActionSteps) == 0 {
		return errorf("at least one action step is required")
	}
	for i, step := range req.ActionSteps {
		if step.Description == "" {
			return errorf("action step %d description is required", i+1)
		}
	}
	switch req.IdentifiedByType {
	case model.IdentifiedByEmployee:
		// IdentifiedByName is deliberately not checked here: it is never
		// trusted for EMPLOYEE and gets overwritten from hr_entity once this
		// validation passes — see handleCreateRisk.
		if req.IdentifiedByEmail == nil || strings.TrimSpace(*req.IdentifiedByEmail) == "" {
			return errorf("identified_by_email is required when identified_by_type is %s", model.IdentifiedByEmployee)
		}
	case model.IdentifiedByExternalPerson, model.IdentifiedByTool:
		if req.IdentifiedByName == nil || strings.TrimSpace(*req.IdentifiedByName) == "" {
			return errorf("identified_by_name is required when identified_by_type is %s", req.IdentifiedByType)
		}
	default:
		return errorf("identified_by_type must be %s, %s, or %s", model.IdentifiedByEmployee, model.IdentifiedByExternalPerson, model.IdentifiedByTool)
	}
	return nil
}

// resolveIdentifiedByEmployee verifies email against hr_entity and returns
// their canonical display name. Used by both handleCreateRisk and
// handleUpdateRisk so identified_by_name is always derived server-side for
// identified_by_type=EMPLOYEE, never taken from the request body — a client
// cannot attribute a risk to an employee it hasn't proven exists.
func (d *Deps) resolveIdentifiedByEmployee(ctx context.Context, email string) (string, error) {
	opt, err := d.Employee.Resolve(ctx, strings.TrimSpace(email))
	if err != nil {
		return "", err
	}
	if opt == nil {
		return "", &apierror.Error{
			StatusCode: http.StatusUnprocessableEntity,
			Body:       "identified_by_email does not match an active WSO2 employee",
		}
	}
	return opt.Name, nil
}

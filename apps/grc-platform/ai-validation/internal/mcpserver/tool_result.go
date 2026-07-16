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

package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createdByAgent is the created_by value for all rows written by this service.
const createdByAgent = "ai-validation-agent"

// maxSummaryChars truncates the stored summary (doc §6).
const maxSummaryChars = 2000

// resultGap is one gap entry in submit_validation_result.
type resultGap struct {
	RequirementAspect string `json:"requirementAspect"`
	Issue             string `json:"issue"`
	Severity          string `json:"severity"`
	FileName          string `json:"fileName,omitempty"`
}

// resultArgs is the input of submit_validation_result — this schema IS the
// output contract of the validation task (doc §4.2.3).
type resultArgs struct {
	Result                       string      `json:"result"`
	Confidence                   *float64    `json:"confidence"`
	Summary                      string      `json:"summary"`
	Gaps                         []resultGap `json:"gaps"`
	Feedback                     []string    `json:"feedback"`
	PreviousSubmissionComparison string      `json:"previousSubmissionComparison"`
}

var validVerdicts = map[string]bool{"PASS": true, "FAIL": true, "UNCERTAIN": true}
var validSeverities = map[string]bool{"HIGH": true, "MEDIUM": true, "LOW": true}

// submitValidationResult validates the verdict server-side, writes the
// terminal advisory row plus an AI_VALIDATED trail entry through the entity,
// and revokes the session. It never touches evidence/control status — results
// are hints only (threat model [04]).
func (s *Server) submitValidationResult(ctx context.Context, sess *Session, raw json.RawMessage) (*mcp.CallToolResult, error) {
	var args resultArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("malformed arguments: " + err.Error()), nil
	}
	if !validVerdicts[args.Result] {
		return toolError("result must be PASS, FAIL, or UNCERTAIN"), nil
	}
	if args.Confidence == nil || *args.Confidence < 0 || *args.Confidence > 1 {
		return toolError("confidence must be a number between 0.0 and 1.0"), nil
	}
	if strings.TrimSpace(args.Summary) == "" {
		return toolError("summary is required"), nil
	}
	for _, g := range args.Gaps {
		if strings.TrimSpace(g.RequirementAspect) == "" || strings.TrimSpace(g.Issue) == "" {
			return toolError("every gap requires requirementAspect and issue"), nil
		}
		if !validSeverities[g.Severity] {
			return toolError("gap severity must be HIGH, MEDIUM, or LOW"), nil
		}
	}

	// previousSubmissionComparison is appended as the summary's final
	// sentence (doc §6); gaps/feedback are stored as JSON array strings.
	summary := strings.TrimSpace(args.Summary)
	if cmp := strings.TrimSpace(args.PreviousSubmissionComparison); cmp != "" {
		summary += " " + cmp
	}
	if len(summary) > maxSummaryChars {
		summary = summary[:maxSummaryChars]
	}
	gapsJSON, err := json.Marshal(args.Gaps)
	if err != nil {
		return toolError("could not encode gaps"), nil
	}
	feedbackJSON, err := json.Marshal(args.Feedback)
	if err != nil {
		return toolError("could not encode feedback"), nil
	}
	gapsStr, feedbackStr := string(gapsJSON), string(feedbackJSON)

	body := entCreateAIValidation{
		ControlID:       sess.Scope.ControlID,
		Result:          args.Result,
		GapsFound:       &gapsStr,
		Feedback:        &feedbackStr,
		Summary:         &summary,
		ConfidenceScore: args.Confidence,
		CreatedBy:       createdByAgent,
	}
	if err := s.entity.Post(ctx, fmt.Sprintf("/evidence/%d/ai-validations", sess.Scope.EvidenceID), body, nil); err != nil {
		return nil, fmt.Errorf("could not record validation result: %w", err)
	}

	// AI_VALIDATED trail entry — best-effort: the verdict row is already
	// written, so a trail failure is logged, not surfaced.
	details, _ := json.Marshal(map[string]any{"result": args.Result, "confidence": *args.Confidence})
	detailsStr := string(details)
	createdBy := createdByAgent
	trail := entCreateTrail{
		ControlID:  &sess.Scope.ControlID,
		EvidenceID: &sess.Scope.EvidenceID,
		Action:     "AI_VALIDATED",
		Details:    &detailsStr,
		CreatedBy:  &createdBy,
	}
	if err := s.entity.Post(ctx, fmt.Sprintf("/audits/%d/trail", sess.Scope.AuditID), trail, nil); err != nil {
		s.log.Error("trail write failed after validation result", "evidenceId", sess.Scope.EvidenceID, "err", err)
	}

	// Single-job token: revoke on successful completion.
	s.store.Revoke(sess.Token)

	s.log.Info("validation result recorded",
		"auditId", sess.Scope.AuditID, "controlId", sess.Scope.ControlID,
		"evidenceId", sess.Scope.EvidenceID, "result", args.Result,
		"confidence", *args.Confidence, "gaps", len(args.Gaps))
	return textResult("verdict recorded — the validation session is complete"), nil
}

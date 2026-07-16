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
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxPreviousSubmissions caps how many prior submissions are included
// (minimum-content principle: the LLM sees only what it needs).
const maxPreviousSubmissions = 3

// maxTrailEntries caps the recent-trail slice in the context payload.
const maxTrailEntries = 10

// Context payload shapes (doc §4.2.1).
type ctxControl struct {
	ControlNumber       string `json:"controlNumber"`
	Description         string `json:"description"`
	EvidenceRequirement string `json:"evidenceRequirement"`
	RequirementType     string `json:"requirementType"`
}

type ctxFile struct {
	FileID        int    `json:"fileId"`
	FileName      string `json:"fileName"`
	FileType      string `json:"fileType,omitempty"`
	FileSizeBytes int64  `json:"fileSizeBytes,omitempty"`
}

type ctxComment struct {
	Author    string `json:"author,omitempty"`
	CreatedAt string `json:"createdAt"`
	Content   string `json:"content"`
}

type ctxSubmission struct {
	EvidenceID     int          `json:"evidenceId"`
	Status         string       `json:"status,omitempty"`
	SubmittedBy    string       `json:"submittedBy,omitempty"`
	SubmittedAt    string       `json:"submittedAt"`
	Files          []ctxFile    `json:"files"`
	ReviewComments []ctxComment `json:"reviewComments,omitempty"`
}

type ctxTrailEntry struct {
	Action    string `json:"action"`
	Actor     string `json:"actor,omitempty"`
	CreatedAt string `json:"createdAt"`
	Details   string `json:"details,omitempty"`
}

type validationContext struct {
	Control             ctxControl      `json:"control"`
	CurrentEvidence     ctxSubmission   `json:"currentEvidence"`
	PreviousSubmissions []ctxSubmission `json:"previousSubmissions"`
	RecentTrail         []ctxTrailEntry `json:"recentTrail"`
}

// getValidationContext assembles all cheap metadata in one call, saving the
// LLM 3-4 round trips. Reviewer comments on prior submissions are filtered to
// external only (is_internal = false) — internal reviewer notes are never
// sent to the LLM (minimum-content principle, threat model [04]).
func (s *Server) getValidationContext(ctx context.Context, sess *Session, _ json.RawMessage) (*mcp.CallToolResult, error) {
	sc := sess.Scope

	// Control (embeds description + evidenceRequirement).
	var control entControl
	if err := s.entity.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d", sc.AuditID, sc.ControlID), &control); err != nil {
		return nil, fmt.Errorf("could not load control: %w", err)
	}

	// All submissions for the control; the scoped evidenceId must be among
	// them (scope sanity check — protocol error otherwise).
	var evResp struct {
		Evidence []entEvidence `json:"evidence"`
	}
	if err := s.entity.Get(ctx, fmt.Sprintf("/audits/%d/controls/%d/evidence", sc.AuditID, sc.ControlID), &evResp); err != nil {
		return nil, fmt.Errorf("could not load evidence list: %w", err)
	}
	var current *entEvidence
	var previous []entEvidence
	for i := range evResp.Evidence {
		e := evResp.Evidence[i]
		switch {
		case e.ID == sc.EvidenceID:
			current = &e
		case e.ID < sc.EvidenceID: // earlier submissions only
			previous = append(previous, e)
		}
	}
	if current == nil {
		return nil, fmt.Errorf("evidence %d does not belong to control %d (scope mismatch)", sc.EvidenceID, sc.ControlID)
	}
	// Newest prior submissions first, capped.
	sort.Slice(previous, func(i, j int) bool { return previous[i].ID > previous[j].ID })
	if len(previous) > maxPreviousSubmissions {
		previous = previous[:maxPreviousSubmissions]
	}

	out := validationContext{
		Control: ctxControl{
			ControlNumber:       control.ControlNumber,
			Description:         control.Description,
			EvidenceRequirement: strOrEmpty(control.EvidenceRequirement),
			RequirementType:     control.RequirementType,
		},
		CurrentEvidence:     s.submission(ctx, *current, false),
		PreviousSubmissions: []ctxSubmission{},
		RecentTrail:         []ctxTrailEntry{},
	}
	for _, e := range previous {
		out.PreviousSubmissions = append(out.PreviousSubmissions, s.submission(ctx, e, true))
	}

	// Recent trail for this control (best-effort — an empty slice on error).
	var trail entListTrail
	if err := s.entity.Get(ctx, fmt.Sprintf("/audits/%d/trail?limit=100&offset=0", sc.AuditID), &trail); err == nil {
		for _, t := range trail.Trail {
			if t.ControlID == nil || *t.ControlID != sc.ControlID {
				continue
			}
			out.RecentTrail = append(out.RecentTrail, ctxTrailEntry{
				Action:    t.Action,
				Actor:     strOrEmpty(t.CreatedBy),
				CreatedAt: t.CreatedOn.UTC().Format(time.RFC3339),
				Details:   strOrEmpty(t.Details),
			})
			if len(out.RecentTrail) >= maxTrailEntries {
				break
			}
		}
	}

	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("could not encode context: %w", err)
	}
	return textResult(string(b)), nil
}

// submission builds one submission entry: file list plus (for prior
// submissions) external-only review comments.
func (s *Server) submission(ctx context.Context, e entEvidence, withComments bool) ctxSubmission {
	sub := ctxSubmission{
		EvidenceID:  e.ID,
		Status:      e.Status,
		SubmittedBy: strOrEmpty(e.CreatedBy),
		SubmittedAt: e.CreatedOn.UTC().Format(time.RFC3339),
		Files:       []ctxFile{},
	}

	var files entListEvidenceFiles
	if err := s.entity.Get(ctx, fmt.Sprintf("/evidence/%d/files", e.ID), &files); err == nil {
		for _, f := range files.Files {
			cf := ctxFile{FileID: f.ID, FileName: f.FileName, FileType: strOrEmpty(f.FileType)}
			if f.FileSize != nil {
				cf.FileSizeBytes = *f.FileSize
			}
			sub.Files = append(sub.Files, cf)
		}
	}

	if withComments {
		var comments entListComments
		if err := s.entity.Get(ctx, fmt.Sprintf("/evidence/%d/comments", e.ID), &comments); err == nil {
			for _, c := range comments.Comments {
				if c.IsInternal { // never send internal reviewer notes to the LLM
					continue
				}
				sub.ReviewComments = append(sub.ReviewComments, ctxComment{
					Author:    strOrEmpty(c.CreatedBy),
					CreatedAt: c.CreatedOn.UTC().Format(time.RFC3339),
					Content:   c.Content,
				})
			}
		}
	}
	return sub
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

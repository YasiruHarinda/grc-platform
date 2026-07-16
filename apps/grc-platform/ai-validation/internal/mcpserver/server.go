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

// Package mcpserver implements the AI validation MCP Server (threat model
// [04]): the only data-access surface the validation agent has. Every tool
// call is authenticated with a job-scoped session token and validated against
// that session's {auditId, controlId, evidenceId} scope. All data flows
// through the Compliance Entity — this component never holds the Azure key.
package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/config"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/mcpserver/entityclient"
)

// Server wires the session store, entity client, and MCP tool handlers.
type Server struct {
	cfg    config.MCPServer
	entity *entityclient.Client
	store  *SessionStore
	log    *slog.Logger
	mcp    *mcp.Server
}

// New constructs the MCP server with all three v1 tools registered.
func New(cfg config.MCPServer, logger *slog.Logger) *Server {
	s := &Server{
		cfg:    cfg,
		entity: entityclient.New(cfg.ComplianceEntityBaseURL),
		store:  NewSessionStore(cfg.SessionTTL),
		log:    logger,
	}

	s.mcp = mcp.NewServer(&mcp.Implementation{Name: "grc-ai-validation", Version: "1.0.0"}, nil)
	s.mcp.AddTool(&mcp.Tool{
		Name:        "get_validation_context",
		Description: "Load the control requirement, the current submission's file list, previous submissions with reviewer feedback, and recent audit trail. Call this first.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, s.tool("get_validation_context", s.getValidationContext))
	s.mcp.AddTool(&mcp.Tool{
		Name:        "get_evidence_file",
		Description: "Fetch the content of one evidence file by fileId. Only files listed by get_validation_context are accessible. PDFs and images are returned natively; spreadsheets are converted to CSV text per sheet. Call once per file you need to inspect.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": { "fileId": { "type": "integer", "description": "fileId from get_validation_context" } },
			"required": ["fileId"],
			"additionalProperties": false
		}`),
	}, s.tool("get_evidence_file", s.getEvidenceFile))
	s.mcp.AddTool(&mcp.Tool{
		Name:        "submit_validation_result",
		Description: "Record the final advisory validation verdict. Call exactly once, after reviewing the requirement and all relevant evidence files. This ends the validation session.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"result":     { "type": "string", "enum": ["PASS", "FAIL", "UNCERTAIN"] },
				"confidence": { "type": "number", "description": "0.0-1.0" },
				"summary":    { "type": "string", "description": "2-4 sentence overall assessment for reviewers" },
				"gaps": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"requirementAspect": { "type": "string" },
							"issue":             { "type": "string" },
							"severity":          { "type": "string", "enum": ["HIGH", "MEDIUM", "LOW"] },
							"fileName":          { "type": "string" }
						},
						"required": ["requirementAspect", "issue", "severity"],
						"additionalProperties": false
					}
				},
				"feedback": {
					"type": "array",
					"description": "Concrete, actionable steps the submitting team can take before compliance review",
					"items": { "type": "string" }
				},
				"previousSubmissionComparison": {
					"type": "string",
					"description": "Empty string if first submission; otherwise whether each prior rejection comment was addressed"
				}
			},
			"required": ["result", "confidence", "summary", "gaps", "feedback", "previousSubmissionComparison"],
			"additionalProperties": false
		}`),
	}, s.tool("submit_validation_result", s.submitValidationResult))

	return s
}

// Handler returns the full HTTP handler: healthz, session bootstrap, lifecycle
// rows, and the MCP Streamable HTTP endpoint at /mcp.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /internal/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /internal/lifecycle", s.handleLifecycle)
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, nil))
	return mux
}

// toolFunc is a session-authenticated tool implementation.
type toolFunc func(ctx context.Context, sess *Session, args json.RawMessage) (*mcp.CallToolResult, error)

// tool wraps a toolFunc with session resolution, per-task allowlist
// enforcement, and structured logging of every call (threat model [04]:
// "all MCP tool calls logged"). File contents / LLM text are never logged.
func (s *Server) tool(name string, fn toolFunc) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		var token string
		if req.Extra != nil && req.Extra.Header != nil {
			token = bearerFromHeader(req.Extra.Header.Get("Authorization"))
		}
		sess := s.store.Resolve(token)
		if sess == nil {
			s.log.Warn("mcp tool call rejected", "tool", name, "reason", "invalid or expired session")
			return nil, errors.New("invalid or expired session token")
		}
		if !sess.AllowedTools[name] {
			s.log.Warn("mcp tool call rejected", "tool", name, "task", sess.Task, "reason", "tool not in task allowlist")
			return nil, fmt.Errorf("tool %q is not allowed for task %q", name, sess.Task)
		}

		res, err := fn(ctx, sess, req.Params.Arguments)

		outcome := "ok"
		switch {
		case err != nil:
			outcome = "protocol_error"
		case res != nil && res.IsError:
			outcome = "tool_error"
		}
		s.log.Info("mcp tool call",
			"tool", name, "task", sess.Task,
			"auditId", sess.Scope.AuditID, "controlId", sess.Scope.ControlID, "evidenceId", sess.Scope.EvidenceID,
			"argsBytes", len(req.Params.Arguments),
			"durationMs", time.Since(start).Milliseconds(),
			"outcome", outcome)
		return res, err
	}
}

// toolError is a content-level failure the model should see and reason about
// (unsupported file, out-of-scope id, oversized content). Infrastructure
// failures return a Go error instead so the agent's retry policy can kick in.
func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// bearerFromHeader extracts a bearer token from an Authorization header value.
func bearerFromHeader(h string) string {
	if len(h) > 7 && (h[:7] == "Bearer " || h[:7] == "bearer ") {
		return h[7:]
	}
	return ""
}

// lifecycleRequest is the body of POST /internal/lifecycle — the agent's
// (non-LLM) path for writing PENDING / ERROR lifecycle rows. Authenticated
// with the shared secret, not a session token, because ERROR rows must be
// writable even after a session expired or was never created.
type lifecycleRequest struct {
	Scope   Scope  `json:"scope"`
	Result  string `json:"result"` // PENDING | ERROR
	Summary string `json:"summary"`
}

func (s *Server) handleLifecycle(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(bearerToken(r)), []byte(s.cfg.MCPSharedSecret)) != 1 {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req lifecycleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, `{"message":"malformed request body"}`, http.StatusBadRequest)
		return
	}
	if req.Result != "PENDING" && req.Result != "ERROR" {
		http.Error(w, `{"message":"result must be PENDING or ERROR"}`, http.StatusBadRequest)
		return
	}
	if req.Scope.ControlID <= 0 || req.Scope.EvidenceID <= 0 {
		http.Error(w, `{"message":"scope requires positive controlId and evidenceId"}`, http.StatusBadRequest)
		return
	}
	var summary *string
	if req.Summary != "" {
		summary = &req.Summary
	}
	body := entCreateAIValidation{
		ControlID: req.Scope.ControlID,
		Result:    req.Result,
		Summary:   summary,
		CreatedBy: createdByAgent,
	}
	if err := s.entity.Post(r.Context(), fmt.Sprintf("/evidence/%d/ai-validations", req.Scope.EvidenceID), body, nil); err != nil {
		s.log.Error("lifecycle row write failed", "result", req.Result, "evidenceId", req.Scope.EvidenceID, "err", err)
		http.Error(w, `{"message":"could not write lifecycle row"}`, http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

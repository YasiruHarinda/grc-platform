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

// Package agent implements the Validation Agent component (threat model [04]):
// the backend triggers it fire-and-forget after an evidence submission; it runs
// an Anthropic tool loop against the MCP server and records an advisory result.
// It holds the ANTHROPIC_API_KEY; it never holds the Azure key or DB access.
package agent

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/agent/mcpclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/agent/task"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/config"
)

// Server is the Validation Agent HTTP server.
type Server struct {
	cfg    config.Agent
	runner *runner
	log    *slog.Logger
	jobs   sync.WaitGroup // tracks in-flight jobs for graceful drain
}

// New constructs the agent server and its job runner.
func New(cfg config.Agent, logger *slog.Logger) *Server {
	client := anthropic.NewClient(option.WithAPIKey(cfg.AnthropicAPIKey))
	mcp := mcpclient.New(cfg.MCPBaseURL, cfg.MCPSharedSecret)
	return &Server{
		cfg: cfg,
		log: logger,
		runner: &runner{
			mcp: mcp,
			loop: &loop{
				client:       client,
				defaultModel: cfg.AnthropicModel,
				maxIter:      cfg.MaxLoopIterations,
				log:          logger,
			},
			timeout:         cfg.ValidationTimeout,
			sessionTTLGrace: time.Minute,
			log:             logger,
		},
	}
}

// Handler returns the agent's HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /api/v1/validations", s.handleTrigger)
	return mux
}

// triggerRequest is the body the backend sends to start a validation.
type triggerRequest struct {
	Task        string     `json:"task"`
	Scope       task.Scope `json:"scope"`
	RequestedBy string     `json:"requestedBy"`
}

// handleTrigger authenticates the backend, validates the task/scope, accepts
// the job (202), and runs it in a tracked background goroutine.
func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(s.cfg.AgentAPIKey)) != 1 {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req triggerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, `{"message":"malformed request body"}`, http.StatusBadRequest)
		return
	}
	spec, ok := task.Get(req.Task)
	if !ok {
		http.Error(w, `{"message":"unknown task"}`, http.StatusBadRequest)
		return
	}
	if !spec.RequiresScope(req.Scope) {
		http.Error(w, `{"message":"scope does not satisfy the task's required keys"}`, http.StatusBadRequest)
		return
	}

	jobID := newJobID()
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("validation job panicked", "jobId", jobID, "recover", rec)
			}
		}()
		s.runner.run(spec, req.Scope, req.RequestedBy, jobID)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": jobID})
}

// Drain waits for in-flight jobs to finish, up to the given timeout.
func (s *Server) Drain(timeout time.Duration) {
	done := make(chan struct{})
	go func() { s.jobs.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		s.log.Warn("drain timed out; abandoning in-flight jobs")
	}
}

// newJobID returns a sortable-ish opaque job id.
func newJobID() string {
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	return "vj_" + hex.EncodeToString(raw)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

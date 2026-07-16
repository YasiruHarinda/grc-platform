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

// Package task holds the agent's task registry — the extensibility seam
// (design §4.1.4). A new AI feature is a new TaskSpec (+ optional MCP tool +
// trigger call site); the loop, bridge, session, and auth code never change.
package task

import "time"

// Scope is the evidence chain a validation job operates on. It is echoed to the
// MCP server, which binds every session token to it and rejects any tool call
// that steps outside it (threat model [04]: no wildcard reads).
type Scope struct {
	AuditID    int `json:"auditId"`
	ControlID  int `json:"controlId"`
	EvidenceID int `json:"evidenceId"`
}

// TaskSpec describes one agent task: its prompt, the MCP tools it may use, the
// scope keys it requires, and per-task loop/model overrides.
type TaskSpec struct {
	Name          string             // e.g. "validate_evidence"
	SystemPrompt  func(Scope) string // per-task prompt builder
	AllowedTools  []string           // MCP tool allowlist (mirrored server-side)
	ScopeRequired []string           // e.g. ["auditId","controlId","evidenceId"]
	Model         string             // optional override; "" → cfg.AnthropicModel
	MaxIterations int                // optional override; 0 → cfg.MaxLoopIterations
	Timeout       time.Duration      // optional override; 0 → cfg.ValidationTimeout
}

// Registry maps a task name to its spec. v1 ships the single validate_evidence
// task; future features register here.
var Registry = map[string]TaskSpec{
	"validate_evidence": {
		Name:          "validate_evidence",
		SystemPrompt:  validateEvidencePrompt,
		AllowedTools:  []string{"get_validation_context", "get_evidence_file", "submit_validation_result"},
		ScopeRequired: []string{"auditId", "controlId", "evidenceId"},
		MaxIterations: 12,
		Timeout:       5 * time.Minute,
	},
}

// Get returns the spec for a task name.
func Get(name string) (TaskSpec, bool) {
	spec, ok := Registry[name]
	return spec, ok
}

// RequiresScope reports whether the given scope satisfies the task's required
// keys (all listed keys must be positive).
func (t TaskSpec) RequiresScope(s Scope) bool {
	for _, key := range t.ScopeRequired {
		switch key {
		case "auditId":
			if s.AuditID <= 0 {
				return false
			}
		case "controlId":
			if s.ControlID <= 0 {
				return false
			}
		case "evidenceId":
			if s.EvidenceID <= 0 {
				return false
			}
		}
	}
	return true
}

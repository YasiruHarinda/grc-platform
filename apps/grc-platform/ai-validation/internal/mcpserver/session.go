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
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Scope is the exact evidence chain a session is allowed to touch. Every tool
// call is validated against it — no wildcard reads (threat model [04]).
type Scope struct {
	AuditID    int `json:"auditId"`
	ControlID  int `json:"controlId"`
	EvidenceID int `json:"evidenceId"`
}

// Session is one job-scoped MCP session, created by the agent via
// POST /internal/sessions and revoked when submit_validation_result succeeds
// (or on TTL expiry).
type Session struct {
	Token        string
	Task         string
	Scope        Scope
	AllowedTools map[string]bool
	RequestedBy  string
	ExpiresAt    time.Time
}

// taskAllowedTools mirrors the agent's task registry allowlists (defense in
// depth: a compromised or buggy agent cannot call out-of-allowlist tools).
var taskAllowedTools = map[string][]string{
	"validate_evidence": {"get_validation_context", "get_evidence_file", "submit_validation_result"},
}

// SessionStore is an in-memory token → session map (single replica; sessions
// are short-lived and job-scoped, so losing them on restart is acceptable).
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewSessionStore constructs a SessionStore and starts the TTL sweep.
func NewSessionStore(ttl time.Duration) *SessionStore {
	s := &SessionStore{sessions: map[string]*Session{}, ttl: ttl}
	go s.sweep()
	return s
}

func (s *SessionStore) sweep() {
	for range time.Tick(time.Minute) {
		now := time.Now()
		s.mu.Lock()
		for tok, sess := range s.sessions {
			if now.After(sess.ExpiresAt) {
				delete(s.sessions, tok)
			}
		}
		s.mu.Unlock()
	}
}

// Create issues a new opaque 256-bit session token for the given task/scope.
func (s *SessionStore) Create(task string, scope Scope, requestedBy string, ttl time.Duration) (*Session, bool) {
	tools, ok := taskAllowedTools[task]
	if !ok {
		return nil, false
	}
	if ttl <= 0 || ttl > s.ttl {
		ttl = s.ttl
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, false
	}
	allowed := make(map[string]bool, len(tools))
	for _, t := range tools {
		allowed[t] = true
	}
	sess := &Session{
		Token:        hex.EncodeToString(raw),
		Task:         task,
		Scope:        scope,
		AllowedTools: allowed,
		RequestedBy:  requestedBy,
		ExpiresAt:    time.Now().Add(ttl),
	}
	s.mu.Lock()
	s.sessions[sess.Token] = sess
	s.mu.Unlock()
	return sess, true
}

// Resolve returns the live session for a bearer token, or nil.
func (s *SessionStore) Resolve(token string) *Session {
	if token == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	return sess
}

// Revoke deletes a session (called when submit_validation_result succeeds).
func (s *SessionStore) Revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// createSessionRequest is the body of POST /internal/sessions.
type createSessionRequest struct {
	Task        string `json:"task"`
	Scope       Scope  `json:"scope"`
	RequestedBy string `json:"requestedBy"`
	TTLSeconds  int    `json:"ttlSeconds"`
}

// handleCreateSession handles POST /internal/sessions, authenticated with the
// MCP_SHARED_SECRET bootstrap bearer (agent-only; component is project-internal).
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(bearerToken(r)), []byte(s.cfg.MCPSharedSecret)) != 1 {
		http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req createSessionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, `{"message":"malformed request body"}`, http.StatusBadRequest)
		return
	}
	if req.Scope.AuditID <= 0 || req.Scope.ControlID <= 0 || req.Scope.EvidenceID <= 0 {
		http.Error(w, `{"message":"scope requires positive auditId, controlId, evidenceId"}`, http.StatusBadRequest)
		return
	}
	sess, ok := s.store.Create(req.Task, req.Scope, req.RequestedBy, time.Duration(req.TTLSeconds)*time.Second)
	if !ok {
		http.Error(w, `{"message":"unknown task"}`, http.StatusBadRequest)
		return
	}
	s.log.Info("mcp session created",
		"task", sess.Task, "auditId", sess.Scope.AuditID,
		"controlId", sess.Scope.ControlID, "evidenceId", sess.Scope.EvidenceID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sessionToken": sess.Token,
		"expiresAt":    sess.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// bearerToken extracts the bearer token from an Authorization header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

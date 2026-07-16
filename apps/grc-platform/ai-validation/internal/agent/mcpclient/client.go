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

// Package mcpclient wraps the agent's two channels to the MCP server:
//   - the bootstrap/lifecycle HTTP API (POST /internal/sessions, /internal/lifecycle),
//     authenticated with the shared secret; and
//   - the scoped MCP tool surface (Streamable HTTP at /mcp), authenticated with
//     the per-job session token issued by bootstrap.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client talks to the MCP server. It holds the shared secret used for the
// agent-only bootstrap/lifecycle endpoints; MCP tool calls use a per-job token.
type Client struct {
	baseURL      string
	sharedSecret string
	http         *http.Client
}

// New constructs a client pointed at the MCP server base URL.
func New(baseURL, sharedSecret string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		sharedSecret: sharedSecret,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

// scope is the wire shape shared by the bootstrap and lifecycle bodies.
type scope struct {
	AuditID    int `json:"auditId"`
	ControlID  int `json:"controlId"`
	EvidenceID int `json:"evidenceId"`
}

type createSessionRequest struct {
	Task        string `json:"task"`
	Scope       scope  `json:"scope"`
	RequestedBy string `json:"requestedBy"`
	TTLSeconds  int    `json:"ttlSeconds"`
}

type createSessionResponse struct {
	SessionToken string `json:"sessionToken"`
	ExpiresAt    string `json:"expiresAt"`
}

// CreateSession bootstraps a scoped MCP session and returns its opaque token.
func (c *Client) CreateSession(ctx context.Context, task string, auditID, controlID, evidenceID int, requestedBy string, ttl time.Duration) (string, error) {
	body := createSessionRequest{
		Task:        task,
		Scope:       scope{AuditID: auditID, ControlID: controlID, EvidenceID: evidenceID},
		RequestedBy: requestedBy,
		TTLSeconds:  int(ttl.Seconds()),
	}
	var out createSessionResponse
	if err := c.postSecret(ctx, "/internal/sessions", body, &out); err != nil {
		return "", err
	}
	if out.SessionToken == "" {
		return "", fmt.Errorf("mcpclient: empty session token")
	}
	return out.SessionToken, nil
}

type lifecycleRequest struct {
	Scope   scope  `json:"scope"`
	Result  string `json:"result"`
	Summary string `json:"summary"`
}

// WriteLifecycle writes a PENDING or ERROR lifecycle row via the MCP server's
// non-LLM path (design §4.2.5). Best-effort: callers log and continue on error.
func (c *Client) WriteLifecycle(ctx context.Context, auditID, controlID, evidenceID int, result, summary string) error {
	body := lifecycleRequest{
		Scope:   scope{AuditID: auditID, ControlID: controlID, EvidenceID: evidenceID},
		Result:  result,
		Summary: summary,
	}
	return c.postSecret(ctx, "/internal/lifecycle", body, nil)
}

// postSecret POSTs JSON with the shared-secret bearer.
func (c *Client) postSecret(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("mcpclient: marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("mcpclient: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.sharedSecret)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("mcpclient: %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("mcpclient: %s responded %d: %s", path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("mcpclient: decode %s response: %w", path, err)
		}
	}
	return nil
}

// Session is a live, scoped MCP tool connection for one job.
type Session struct {
	cs    *mcp.ClientSession
	tools []*mcp.Tool
}

// Connect opens the Streamable HTTP MCP connection using the session token and
// lists the tools the server exposes for this session.
func (c *Client) Connect(ctx context.Context, token string) (*Session, error) {
	httpClient := &http.Client{Transport: bearerRoundTripper{base: http.DefaultTransport, token: token}}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             c.baseURL + "/mcp",
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true, // request/response only; no server-initiated stream needed
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "grc-ai-validation-agent", Version: "1.0.0"}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: connect: %w", err)
	}
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		_ = cs.Close()
		return nil, fmt.Errorf("mcpclient: list tools: %w", err)
	}
	return &Session{cs: cs, tools: res.Tools}, nil
}

// Tools returns the MCP tool definitions available for this session.
func (s *Session) Tools() []*mcp.Tool { return s.tools }

// CallTool invokes an MCP tool with raw JSON arguments.
func (s *Session) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	var arguments any = args
	if len(args) == 0 {
		arguments = map[string]any{}
	}
	return s.cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
}

// Close ends the MCP session connection.
func (s *Session) Close() error {
	if s.cs == nil {
		return nil
	}
	return s.cs.Close()
}

// bearerRoundTripper injects the session token on every MCP HTTP request.
type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (b bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(clone)
}

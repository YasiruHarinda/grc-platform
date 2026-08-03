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

// Package emailer is a thin client to the shared email-sending service
// (email-service). The service's own code has no inbound auth check, but the
// real Choreo-hosted instance sits behind API Manager with OAuth2
// client-credentials — same shape as the hrentity client — so this client
// fetches and caches its own bearer token, refreshing it once it's within
// tokenExpiryBuffer of expiring.
package emailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client posts send-email requests to the email-service.
type Client struct {
	baseURL      string
	from         string
	tokenURL     string
	clientID     string
	clientSecret string
	http         *http.Client

	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// tokenExpiryBuffer is subtracted from the token's reported lifetime so a
// near-expiry token is never handed to an in-flight request.
const tokenExpiryBuffer = 30 * time.Second

// sendAttempts bounds the retry in SendRiskCreated. The Choreo-hosted
// email-service scales to zero when idle, and the first request after that has
// to wait out a cold start that regularly outlasts this client's 10s timeout —
// observed after both a 2-day and a 3-day idle gap, each time a clean 10s
// "context deadline exceeded" with no email sent. The first attempt is what
// wakes the container, so the second one lands on a warm instance and there is
// nothing for a third to fix: hence 2, retried immediately with no backoff.
const sendAttempts = 2

// New constructs a client pointed at the email-service base URL, sending as
// from, authenticating via OAuth2 client-credentials at tokenURL using
// clientID and clientSecret.
func New(baseURL, from, tokenURL, clientID, clientSecret string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		from:         from,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: 10 * time.Second},
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// accessToken returns a valid bearer token for the email-service, reusing the
// cached one until it's close to expiry.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("emailer: build token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("emailer: call token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("emailer: token endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("emailer: decode token response: %w", err)
	}

	c.cachedToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - tokenExpiryBuffer)
	return c.cachedToken, nil
}

// sendEmailRequest is the body of POST /send-email. Template is a
// base64-encoded HTML string — the service does no variable substitution of
// its own, so callers must render the full body before encoding it.
type sendEmailRequest struct {
	To       []string `json:"to"`
	From     string   `json:"from"`
	Subject  string   `json:"subject"`
	Template string   `json:"template"`
}

// responseMessage is the shape of both success and error bodies.
type responseMessage struct {
	Message string `json:"message"`
}

// RiskCreated holds the fields needed to notify a risk owner that a new risk
// has been assigned to them.
type RiskCreated struct {
	RiskCode       string
	RiskTitle      string
	SourceRegister string
	RiskLevel      string
	CreatedBy      string
	DetailURL      string
}

// riskCreatedTemplate renders RiskCreated into the email body. html/template
// (not text/template) is used deliberately so free-text fields like RiskTitle
// are escaped for HTML/URL context.
var riskCreatedTemplate = template.Must(template.New("riskCreated").Parse(`<html>
<body style="font-family: Arial, sans-serif; font-size: 14px; color: #1a1a1a;">
<p>A new risk has been created and assigned to you.</p>
<table cellpadding="4" cellspacing="0">
<tr><td><strong>Risk Code:</strong></td><td>{{.RiskCode}}</td></tr>
<tr><td><strong>Title:</strong></td><td>{{.RiskTitle}}</td></tr>
<tr><td><strong>Source Register:</strong></td><td>{{.SourceRegister}}</td></tr>
<tr><td><strong>Risk Level:</strong></td><td>{{.RiskLevel}}</td></tr>
<tr><td><strong>Created by:</strong></td><td>{{.CreatedBy}}</td></tr>
</table>
<p><a href="{{.DetailURL}}">View risk</a></p>
</body>
</html>`))

// SendRiskCreated notifies ownerEmail that a risk was created and assigned to
// them. ownerEmail is the only recipient — no cc/bcc are ever sent, by design.
// Retries once on a transport failure to ride out an email-service cold start;
// see sendAttempts. Callers must expect this to block for up to two full client
// timeouts and so should not run it on a request path.
func (c *Client) SendRiskCreated(ctx context.Context, ownerEmail string, info RiskCreated) error {
	var body bytes.Buffer
	if err := riskCreatedTemplate.Execute(&body, info); err != nil {
		return fmt.Errorf("emailer: render template: %w", err)
	}

	reqBody := sendEmailRequest{
		To:       []string{ownerEmail},
		From:     c.from,
		Subject:  fmt.Sprintf("New Risk Assigned: %s - %s", info.RiskCode, info.RiskTitle),
		Template: base64.StdEncoding.EncodeToString(body.Bytes()),
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("emailer: marshal request: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= sendAttempts; attempt++ {
		retryable, err := c.sendOnce(ctx, b)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || attempt == sendAttempts {
			break
		}
		slog.Warn("emailer: send failed, retrying",
			"attempt", attempt, "of", sendAttempts, "err", err)
	}
	return lastErr
}

// sendOnce makes a single POST /send-email attempt. retryable is true only
// when the request never produced a response at all — a timeout or a
// connection-level failure, the shape a cold start takes. Anything the service
// actually answered with is a verdict rather than a hiccup (a 400 for an empty
// recipient or bad base64, a 401/403 for credentials that aren't subscribed to
// this API) and would fail identically on a second try, so it is not retried.
func (c *Client) sendOnce(ctx context.Context, body []byte) (retryable bool, err error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return false, fmt.Errorf("emailer: get access token: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/send-email", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("emailer: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return true, fmt.Errorf("emailer: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		var msg responseMessage
		_ = json.Unmarshal(raw, &msg)
		if msg.Message != "" {
			return false, fmt.Errorf("emailer: email-service returned %d: %s", resp.StatusCode, msg.Message)
		}
		return false, fmt.Errorf("emailer: email-service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return false, nil
}

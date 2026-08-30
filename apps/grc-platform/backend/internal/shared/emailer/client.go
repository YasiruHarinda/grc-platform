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
	// enabled is the master switch (config EMAIL_NOTIFICATIONS_ENABLED). When
	// false, SendRiskEvent and SendAuditEvent return nil immediately — before
	// any token fetch or HTTP call — so no module sends any email. Callers are
	// unchanged: a disabled send looks exactly like a successful one.
	enabled bool

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
// clientID and clientSecret. When enabled is false every send is a no-op (see
// the Client.enabled field) and the connection fields may be empty.
func New(baseURL, from, tokenURL, clientID, clientSecret string, enabled bool) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		from:         from,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		enabled:      enabled,
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

// maxSubjectRunes caps the subject well under any mail-header line-length
// concern, while staying long enough to show the risk code and a readable
// slice of the title.
const maxSubjectRunes = 200

// sanitizeSubject collapses embedded newlines to spaces and caps the length.
// Unlike the body — rendered through html/template specifically so free-text
// fields are escaped for their context — every subject is built by plain
// fmt.Sprintf over fields like RiskTitle (a 500-character free-text column),
// so nothing else stands between user input and the raw request this client
// sends. Runes rather than bytes, so truncation can't split a multi-byte
// character.
func sanitizeSubject(s string) string {
	// Collapse a CRLF pair to a single space before mapping any leftover lone
	// \r or \n (Mac-classic or Unix-only line endings) — otherwise each half
	// of a \r\n pair would become its own space.
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, s)
	if runes := []rune(s); len(runes) > maxSubjectRunes {
		s = string(runes[:maxSubjectRunes])
	}
	return s
}

// RiskEvent identifies a point in the risk lifecycle that notifies someone.
// The value is only ever used to look up a template — it is never persisted, so
// renaming one is safe.
type RiskEvent string

const (
	EventCreated             RiskEvent = "CREATED"
	EventPendingMgmtApproval RiskEvent = "PENDING_MGMT_APPROVAL"
	EventComplianceApproved  RiskEvent = "COMPLIANCE_APPROVED"
	EventActionPlanCompleted RiskEvent = "ACTION_PLAN_COMPLETED"
	EventPendingOwnerClosure RiskEvent = "PENDING_OWNER_CLOSURE"
	EventPendingMgmtClosure  RiskEvent = "PENDING_MGMT_CLOSURE"
	EventRejected            RiskEvent = "REJECTED"
	EventEscalated           RiskEvent = "ESCALATED"
	EventEscalationCommented RiskEvent = "ESCALATION_COMMENTED"
	// EventEscalatedLead notifies the Risk Assigner's and/or Action Owner's
	// lead (their HR line manager, frozen on the escalation row as an Asgardeo
	// id at escalation time) that a risk they sit above has been escalated.
	// Sent as its own message, separate from EventEscalated's internal
	// recipients, and only when LEAD_ESCALATION_EMAILS_ENABLED is on. Purely
	// informational: it declares no actions, so the "Who needs to act" block is
	// absent — a lead is being told, not tasked. The audit-module counterpart
	// is AuditEventReminderOverdueLead.
	EventEscalatedLead RiskEvent = "ESCALATED_LEAD"
	// EventPendingComplianceReview and EventPendingComplianceClosure notify
	// the Compliance Admin role — not a named individual, see
	// notifyComplianceAdmins — that a risk has cleared owner/management
	// approval and is now sitting in PENDING_COMPLIANCE_REVIEW or
	// PENDING_COMPLIANCE_CLOSURE waiting on them. Distinct from
	// EventPendingMgmtApproval/EventPendingMgmtClosure, whose "Who needs to
	// act" block hardcodes the Management Approver: sending those to
	// compliance admins instead named the wrong role and the wrong person in
	// the email body, even though the recipient list was correct.
	EventPendingComplianceReview  RiskEvent = "PENDING_COMPLIANCE_REVIEW"
	EventPendingComplianceClosure RiskEvent = "PENDING_COMPLIANCE_CLOSURE"
	// EventClosed fires once, when Compliance closes the risk
	// (PENDING_COMPLIANCE_CLOSURE → CLOSED). Purely informational — nothing is
	// left to do, so unlike every other event it declares no actions — and
	// sent the same two-call way as EventEscalated: notifyRiskEvent to the
	// three named roles, notifyComplianceAdmins for the role-wide one.
	EventClosed RiskEvent = "CLOSED"
)

// RiskEventInfo carries everything any template might render. Fields not
// relevant to a given event are left zero and the shared body omits them, which
// is why there is one struct rather than one per event.
type RiskEventInfo struct {
	RiskCode       string
	RiskTitle      string
	SourceRegister string
	RiskLevel      string
	// Actor is whoever performed the action that triggered this email — the
	// person who created, approved, rejected, or completed something. It is
	// NOT the risk's assigner or owner, which are permanent fields on the risk;
	// the same person may appear here for one event and not the next. Each
	// event labels it for itself (see eventTemplate.actorLabel), because
	// "Created by" and "Rejected by" are not interchangeable.
	Actor string
	// Comment carries a rejection reason or an escalation note. Omitted from
	// the body when empty.
	Comment   string
	DetailURL string
	// People maps a role name (RoleRiskAssigner and friends) to the person
	// filling it, already formatted as "Display Name (email)". Only the roles
	// this event declares in its template are read; anything else is ignored,
	// and a role with no entry is skipped rather than rendered blank.
	//
	// The event owns *what* each role must do; the caller owns *who* fills it.
	// That split keeps the instruction copy here, next to the subject and lead,
	// instead of scattered across handlers.
	People map[string][]string
}

// Role names used as keys in RiskEventInfo.People. They are display strings —
// they appear verbatim in the email — and are constants only so a caller and a
// template can't disagree on spelling.
const (
	RoleRiskAssigner       = "Risk Assigner"
	RoleRiskOwner          = "Risk Owner"
	RoleManagementApprover = "Management Approver"
	RoleActionOwner        = "Action Owner"
)

// roleInstruction is one line of the "Who needs to act" block: a role and what
// that role is expected to do next for this particular event.
type roleInstruction struct {
	role        string
	instruction string
}

// renderedAction is a roleInstruction joined to the actual people filling it,
// ready for the template.
type renderedAction struct {
	Role        string
	People      []string
	Instruction string
}

// riskCreated is retained as an alias so the original call site and its tests
// keep compiling; new code should use RiskEventInfo directly.
type RiskCreated = RiskEventInfo

// eventTemplate is the per-event copy. Everything structural (the field table,
// the comment block, the link) lives in bodyTemplate below, so adding an event
// is a lead sentence rather than another HTML blob.
//
// There is deliberately no per-event subject here anymore: every event for a
// given risk shares one subject ("[GRC Platform] {RiskCode} - {RiskTitle}",
// built once in SendRiskEvent) so a recipient's several emails about the same
// risk land in one thread in their mail client — Gmail and Outlook both group
// by an exact subject match when there is no Message-ID chain to follow, and
// the email-service this platform calls gives no way to set one (see
// sendEmailRequest). A varying subject per event, however descriptive,
// defeats that grouping.
type eventTemplate struct {
	lead string
	// actorLabel names what Actor did, for this event specifically — "Created
	// by" on creation, "Rejected by" on a rejection. A single generic label
	// would leave the reader guessing which of the several people on a risk
	// the address belongs to.
	actorLabel string
	// actions lists, in order, the roles expected to act once this event has
	// fired, and what each must do. Empty for events that are purely
	// informational.
	actions []roleInstruction
}

// eventTemplates is the single place to see everything the platform sends.
// A RiskEvent with no entry here is a programming error and SendRiskEvent
// rejects it rather than sending a blank email.
var eventTemplates = map[RiskEvent]eventTemplate{
	EventCreated: {
		lead:       "A new risk has been created and assigned to you.",
		actorLabel: "Created by",
		actions: []roleInstruction{
			{RoleRiskOwner, "Review this risk and approve or reject it."},
		},
	},
	EventPendingMgmtApproval: {
		lead:       "This risk has been approved by its Risk Owner and now needs your management approval.",
		actorLabel: "Approved by Risk Owner",
		actions: []roleInstruction{
			{RoleManagementApprover, "Review this risk and approve or reject the Accept treatment."},
		},
	},
	EventComplianceApproved: {
		lead:       "This risk has cleared compliance approval. Remediation can now begin.",
		actorLabel: "Approved by Compliance",
		actions: []roleInstruction{
			{RoleActionOwner, "Complete the action plan steps, then mark the plan complete."},
			{RoleRiskAssigner, "Track remediation progress, then reassess and submit for approval."},
		},
	},
	EventActionPlanCompleted: {
		lead:       "An action plan for this risk has been completed. Please reassess and submit it for approval.",
		actorLabel: "Completed by",
		actions: []roleInstruction{
			{RoleRiskAssigner, "Reassess the risk and submit it for approval."},
		},
	},
	EventPendingOwnerClosure: {
		lead:       "Remediation for this risk has been marked complete and needs your sign-off as Risk Owner.",
		actorLabel: "Submitted by",
		actions: []roleInstruction{
			{RoleRiskOwner, "Review the completed remediation and approve or reject the closure."},
		},
	},
	EventPendingMgmtClosure: {
		lead:       "This risk's completed remediation has been signed off by its Risk Owner and now needs your management approval before closure.",
		actorLabel: "Approved by Risk Owner",
		actions: []roleInstruction{
			{RoleManagementApprover, "Review the completed remediation and approve or reject the closure."},
		},
	},
	EventPendingComplianceReview: {
		lead:       "This risk has cleared owner/management approval and now needs compliance review before remediation can begin.",
		actorLabel: "Approved by",
		// No actions: compliance approval is role-wide, not a named
		// individual (see canOverrideAssigneeIn) — there is nobody to list
		// under "Who needs to act", and every recipient already knows why
		// they received this.
	},
	EventPendingComplianceClosure: {
		lead:       "This risk's completed remediation has cleared owner/management sign-off and now needs compliance closure.",
		actorLabel: "Approved by",
	},
	EventRejected: {
		lead:       "This risk has been rejected and returned to you for revision.",
		actorLabel: "Rejected by",
		actions: []roleInstruction{
			{RoleRiskAssigner, "Review the comment above, make the required changes, and resubmit."},
		},
	},
	EventEscalationCommented: {
		lead:       "This escalated risk has been reviewed and returned to you. It stays in the Overdue tab until you submit it for completion approval.",
		actorLabel: "Reviewed by",
		actions: []roleInstruction{
			{RoleRiskAssigner, "Act on the comment above, then reassess and submit for completion approval."},
		},
	},
	EventEscalated: {
		lead:       "This risk has passed its implementation date without completing remediation and has been escalated.",
		actorLabel: "Escalated by",
		actions: []roleInstruction{
			{RoleActionOwner, "Complete the outstanding action plan steps."},
			{RoleRiskAssigner, "Bring remediation back on track and keep the risk updated."},
		},
	},
	EventEscalatedLead: {
		lead:       "A risk assigned to someone who reports to you has passed its implementation date without completing remediation and has been escalated.",
		actorLabel: "Escalated by",
		// No actions: a lead is being informed, not assigned a step. The
		// "Who needs to act" block is omitted entirely.
	},
	EventClosed: {
		lead:       "This risk has cleared compliance closure and is now closed. No further action is required.",
		actorLabel: "Closed by",
		// No actions: the risk is done, so there is nobody left to list under
		// "Who needs to act".
	},
}

// bodyTemplate renders the shared body for every event. html/template (not
// text/template) is used deliberately so free-text fields like RiskTitle and
// Comment are escaped for HTML/URL context — several of them are user-supplied.
// bodyTemplate renders the shared body for every event.
//
// Deliberately old-fashioned HTML: tables for layout, inline styles, no
// stylesheet. Email clients (Outlook especially) ignore <style> blocks, drop
// flexbox and grid, and collapse margins unpredictably — tables with explicit
// widths and valign are the only layout that renders consistently across them.
//
// html/template (not text/template) is used so free-text fields like RiskTitle
// and Comment are escaped for HTML/URL context; several are user-supplied.
var bodyTemplate = template.Must(template.New("riskEvent").Parse(`<html>
<body style="margin:0; padding:0; background-color:#f4f5f7;">
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#f4f5f7; padding:24px 12px;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" border="0" style="max-width:600px; background-color:#ffffff; border:1px solid #e1e4e8; border-radius:6px; font-family:Arial,Helvetica,sans-serif; font-size:14px; color:#1a1a1a;">

<tr><td style="padding:20px 24px 8px 24px; font-size:15px; line-height:1.5;">{{.Lead}}</td></tr>

<tr><td style="padding:8px 24px 4px 24px;">
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="font-size:14px;">
<tr>
<td width="170" valign="top" style="padding:6px 12px 6px 0; color:#57606a;">Risk Code</td>
<td valign="top" style="padding:6px 0; font-weight:bold;">{{.Info.RiskCode}}</td>
</tr>
<tr>
<td width="170" valign="top" style="padding:6px 12px 6px 0; color:#57606a;">Title</td>
<td valign="top" style="padding:6px 0;">{{.Info.RiskTitle}}</td>
</tr>
<tr>
<td width="170" valign="top" style="padding:6px 12px 6px 0; color:#57606a;">Source Register</td>
<td valign="top" style="padding:6px 0;">{{.Info.SourceRegister}}</td>
</tr>
<tr>
<td width="170" valign="top" style="padding:6px 12px 6px 0; color:#57606a;">Risk Level</td>
<td valign="top" style="padding:6px 0;">{{.Info.RiskLevel}}</td>
</tr>
{{if .Info.Actor}}<tr>
<td width="170" valign="top" style="padding:6px 12px 6px 0; color:#57606a;">{{.ActorLabel}}</td>
<td valign="top" style="padding:6px 0;">{{.Info.Actor}}</td>
</tr>{{end}}
</table>
</td></tr>

{{if .Info.Comment}}<tr><td style="padding:12px 24px 4px 24px;">
<table width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="padding:12px 14px; background-color:#fff8e1; border-left:3px solid #f0ad4e; font-size:14px; line-height:1.5;">
<span style="color:#57606a;">Comment</span><br>{{.Info.Comment}}
</td></tr>
</table>
</td></tr>{{end}}

{{if .Actions}}<tr><td style="padding:16px 24px 4px 24px;">
<div style="font-weight:bold; font-size:14px; padding-bottom:8px; border-bottom:1px solid #e1e4e8;">Who needs to act</div>
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="font-size:14px;">
{{range .Actions}}<tr>
<td width="170" valign="top" style="padding:10px 12px 10px 0; font-weight:bold;">{{.Role}}</td>
<td valign="top" style="padding:10px 0;">
{{range .People}}<div>{{.}}</div>{{end}}
<div style="color:#57606a; padding-top:2px;">{{.Instruction}}</div>
</td>
</tr>{{end}}
</table>
</td></tr>{{end}}

<tr><td style="padding:20px 24px 24px 24px;">
<a href="{{.Info.DetailURL}}" style="display:inline-block; padding:10px 20px; background-color:#ff7300; color:#ffffff; text-decoration:none; border-radius:4px; font-weight:bold; font-size:14px;">View risk</a>
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>`))

// SendRiskCreated is the original single-purpose entry point, kept so the
// risk-creation path and its tests are unaffected by the move to SendRiskEvent.
func (c *Client) SendRiskCreated(ctx context.Context, ownerEmail string, info RiskCreated) error {
	return c.SendRiskEvent(ctx, EventCreated, []string{ownerEmail}, info)
}

// SendRiskEvent notifies every address in to about ev. All recipients go in a
// single To header rather than one message each: the email-service scales to
// zero, so one send per recipient would multiply the cold-start cost and, for
// an escalation's several recipients, could outlast the caller's timeout.
// Recipients therefore see each other, which is intended for an internal
// notification. No cc/bcc are ever sent.
//
// Retries once on a transport failure to ride out a cold start; see
// sendAttempts. Callers must expect this to block for up to two full client
// timeouts and so should not run it on a request path.
func (c *Client) SendRiskEvent(ctx context.Context, ev RiskEvent, to []string, info RiskEventInfo) error {
	if !c.enabled {
		slog.Info("emailer: notifications disabled, skipping risk send", "event", ev)
		return nil
	}
	tpl, ok := eventTemplates[ev]
	if !ok {
		return fmt.Errorf("emailer: no template for event %q", ev)
	}
	// Drop blanks rather than handing the service an empty recipient, which it
	// answers with a 400 that the retry logic correctly refuses to repeat.
	recipients := make([]string, 0, len(to))
	for _, addr := range to {
		if strings.TrimSpace(addr) != "" {
			recipients = append(recipients, addr)
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("emailer: no recipients for event %q", ev)
	}

	// Join the event's declared roles to the people the caller supplied. A role
	// nobody fills is dropped rather than rendered as an empty row — a risk may
	// legitimately have no action owner yet.
	rendered := make([]renderedAction, 0, len(tpl.actions))
	for _, a := range tpl.actions {
		people := info.People[a.role]
		if len(people) == 0 {
			continue
		}
		rendered = append(rendered, renderedAction{Role: a.role, People: people, Instruction: a.instruction})
	}

	var body bytes.Buffer
	if err := bodyTemplate.Execute(&body, struct {
		Lead       string
		ActorLabel string
		Actions    []renderedAction
		Info       RiskEventInfo
	}{tpl.lead, tpl.actorLabel, rendered, info}); err != nil {
		return fmt.Errorf("emailer: render template: %w", err)
	}

	// One subject per risk, not per event: every event about the same risk
	// reuses it exactly, so a recipient's several emails about it land in one
	// thread in their mail client — see eventTemplate's doc comment.
	subject := sanitizeSubject(fmt.Sprintf("[GRC Platform] %s - %s", info.RiskCode, info.RiskTitle))

	reqBody := sendEmailRequest{
		To:       recipients,
		From:     c.from,
		Subject:  subject,
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

// ActionRoles returns the roles ev's template will render in its "Who needs to
// act" block, in order. Callers use it to resolve only the people that event
// actually needs, instead of looking up every role on every risk.
func ActionRoles(ev RiskEvent) []string {
	tpl, ok := eventTemplates[ev]
	if !ok {
		return nil
	}
	roles := make([]string, 0, len(tpl.actions))
	for _, a := range tpl.actions {
		roles = append(roles, a.role)
	}
	return roles
}

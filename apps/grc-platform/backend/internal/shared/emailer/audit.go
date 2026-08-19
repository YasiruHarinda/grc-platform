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

package emailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
)

// AuditEvent identifies a point in the audit lifecycle that notifies someone.
// The value is only ever used to look up a template — it is never persisted,
// so renaming one is safe.
type AuditEvent string

const (
	// AuditEventOwnerAssigned covers both control-owner and population-owner
	// assignment as one event, not two — the caller batches every item a
	// single owner was just assigned (control and/or population) into one
	// AuditEventInfo.Items and sends one email, so a person who is both a
	// control's and its population's owner in the same request gets exactly
	// one email, not two.
	AuditEventOwnerAssigned      AuditEvent = "AUDIT_OWNER_ASSIGNED"
	AuditEventAuditorAssigned    AuditEvent = "AUDIT_AUDITOR_ASSIGNED"
	AuditEventReminderDue10      AuditEvent = "AUDIT_REMINDER_DUE_10"
	AuditEventReminderDue5       AuditEvent = "AUDIT_REMINDER_DUE_5"
	AuditEventReminderOverdue    AuditEvent = "AUDIT_REMINDER_OVERDUE"
	AuditEventResubmissionNeeded AuditEvent = "AUDIT_RESUBMISSION_NEEDED"
	AuditEventSampleSubmitted    AuditEvent = "AUDIT_SAMPLE_SUBMITTED"

	// The six below notify admin/auditor recipients when a control reaches a
	// given status, regardless of which endpoint produced the transition —
	// see handler/notify.go's notifyControlStatusReached. None ever populate
	// AuditEventInfo.Comment: an internal reviewer's comment must never reach
	// an external auditor, who lacks AUDIT_VIEW_INTERNAL_COMMENTS.
	AuditEventEvidenceInternalReview         AuditEvent = "AUDIT_EVIDENCE_INTERNAL_REVIEW"
	AuditEventPopulationInternalReview       AuditEvent = "AUDIT_POPULATION_INTERNAL_REVIEW"
	AuditEventEvidenceUnderValidation        AuditEvent = "AUDIT_EVIDENCE_UNDER_VALIDATION"
	AuditEventPopulationUnderValidation      AuditEvent = "AUDIT_POPULATION_UNDER_VALIDATION"
	AuditEventPopulationCompleteSampleNeeded AuditEvent = "AUDIT_POPULATION_COMPLETE_SAMPLE_NEEDED"
	AuditEventControlComplete                AuditEvent = "AUDIT_CONTROL_COMPLETE"

	// AuditEventCommentAdded notifies a control's owner(s), admins, and (for a
	// non-internal comment) its assigned auditor that someone commented.
	// Unlike the six above, this one does set Comment — see
	// handler/notify.go's notifyCommentAdded for the internal-visibility gate.
	AuditEventCommentAdded AuditEvent = "AUDIT_COMMENT_ADDED"
)

// AuditEventItem is one control or population round an AuditEventInfo email
// is about. Owner-assignment and reminder-digest emails carry more than one;
// resubmission and sample-submitted emails always carry exactly one.
type AuditEventItem struct {
	ControlNumber string
	Description   string
	DueDate       string // "" if not applicable
	// Tier is "Due in 10 days" | "Due in 5 days" | "Overdue" — reminder digest only.
	Tier string
	// RequirementType is "Evidence Requirement" | "Population Requirement" —
	// labels which requirement this item is about.
	RequirementType string
}

// AuditEventInfo carries everything any audit template might render. Unlike
// RiskEventInfo (one subject, many role-holders, identical content), every
// audit event already has exactly one resolved recipient by the time it
// reaches SendAuditEvent — Items is that recipient's personalized batch.
type AuditEventInfo struct {
	AuditName string
	// Actor is whoever triggered this event — who assigned the owner, who
	// rejected, who submitted the sample. Empty for the system-generated
	// reminder digest, which has no actor.
	Actor string
	// Comment carries a rejection reason. Omitted from the body when empty.
	Comment   string
	DetailURL string
	Items     []AuditEventItem
	// ShowStatus renders the table's Status column. Only the reminder digest
	// sets this: it's the only email whose Items mix tiers (an owner's items
	// due across all three tiers in one email), so Status is the only place
	// to tell them apart. Every other event's items share one implicit status
	// (whatever the event itself already says in Lead), so an empty column
	// would just be dead width — see the "Status column only used in overdue
	// ones" note.
	ShowStatus bool
}

// auditEventTemplate is the per-event copy — the audit equivalent of
// eventTemplate, minus the risk template's per-role "Who needs to act" block:
// an audit event already has exactly one recipient (whoever Items is about),
// so there is nothing left to resolve.
type auditEventTemplate struct {
	subject func(AuditEventInfo) string
	lead    string
	// actorLabel names what Actor did, for this event specifically. Empty for
	// events with no actor (the reminder digest).
	actorLabel string
}

// controlThreadSubject is the shared subject line for every control-tied
// audit email — old and new alike. There's no real Message-ID/In-Reply-To
// transport, so a mail client can only group messages that share one literal
// subject string; this stays stable for a control's whole lifecycle, spanning
// both population and evidence phases.
//
// Only meaningful when Info is about exactly one control (Items has one
// entry) — the common case for every event that uses it. A multi-item
// owner-assigned batch (someone assigned owner of more than one
// control/population at once) isn't "one control"'s thread either, so it
// falls back to the audit-only subject.
func controlThreadSubject(i AuditEventInfo) string {
	if len(i.Items) != 1 {
		return fmt.Sprintf("[GRC Platform] %s", i.AuditName)
	}
	return fmt.Sprintf("[GRC Platform] %s - %s", i.AuditName, i.Items[0].ControlNumber)
}

func reminderSubject(tier string) func(AuditEventInfo) string {
	return func(i AuditEventInfo) string {
		return fmt.Sprintf("Audit Reminder — %s: %d item(s) due", tier, len(i.Items))
	}
}

// auditEventTemplates is the single place to see everything the audit module
// sends. An AuditEvent with no entry here is a programming error and
// SendAuditEvent rejects it rather than sending a blank email.
var auditEventTemplates = map[AuditEvent]auditEventTemplate{
	AuditEventOwnerAssigned: {
		subject:    controlThreadSubject,
		lead:       "You have been assigned as owner on the following item(s).",
		actorLabel: "Assigned by",
	},
	AuditEventAuditorAssigned: {
		subject:    controlThreadSubject,
		lead:       "You have been assigned as the auditor POC on the following item(s).",
		actorLabel: "Assigned by",
	},
	AuditEventReminderDue10: {
		subject:    reminderSubject("Due in 10 days"),
		lead:       "The following item(s) you own are due in 10 days.",
		actorLabel: "",
	},
	AuditEventReminderDue5: {
		subject:    reminderSubject("Due in 5 days"),
		lead:       "The following item(s) you own are due in 5 days.",
		actorLabel: "",
	},
	AuditEventReminderOverdue: {
		subject:    reminderSubject("Overdue"),
		lead:       "The following item(s) you own are overdue.",
		actorLabel: "",
	},
	AuditEventResubmissionNeeded: {
		subject:    controlThreadSubject,
		lead:       "This item was rejected and needs to be resubmitted.",
		actorLabel: "Rejected by",
	},
	AuditEventSampleSubmitted: {
		subject:    controlThreadSubject,
		lead:       "A sample has been submitted for this control. Please submit evidence.",
		actorLabel: "Submitted by",
	},

	// Facts only — never set Comment.
	AuditEventEvidenceInternalReview: {
		subject:    controlThreadSubject,
		lead:       "Evidence has been submitted and is awaiting internal review.",
		actorLabel: "Submitted by",
	},
	AuditEventPopulationInternalReview: {
		subject:    controlThreadSubject,
		lead:       "A population has been submitted and is awaiting internal review.",
		actorLabel: "Submitted by",
	},
	AuditEventEvidenceUnderValidation: {
		subject:    controlThreadSubject,
		lead:       "Evidence has passed internal review and is ready for your validation.",
		actorLabel: "Reviewed by",
	},
	AuditEventPopulationUnderValidation: {
		subject:    controlThreadSubject,
		lead:       "A population has passed internal review and is ready for your validation.",
		actorLabel: "Reviewed by",
	},
	AuditEventPopulationCompleteSampleNeeded: {
		subject:    controlThreadSubject,
		lead:       "The population has been validated. Please submit a sample for evidence collection.",
		actorLabel: "Validated by",
	},
	AuditEventControlComplete: {
		subject:    controlThreadSubject,
		lead:       "This control has been validated and is now complete.",
		actorLabel: "Validated by",
	},
	AuditEventCommentAdded: {
		subject:    controlThreadSubject,
		lead:       "A new comment has been added to this control.",
		actorLabel: "Commented by",
	},
}

// auditBodyTemplate renders the shared body for every audit event. Same
// old-fashioned Outlook-safe table layout as bodyTemplate (see its comment),
// and html/template for the same reason: several fields (Description,
// Comment) are user-supplied free text.
var auditBodyTemplate = template.Must(template.New("auditEvent").Parse(`<html>
<body style="margin:0; padding:0; background-color:#f4f5f7;">
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#f4f5f7; padding:24px 12px;">
<tr><td align="center">
<table width="680" cellpadding="0" cellspacing="0" border="0" style="max-width:680px; background-color:#ffffff; border:1px solid #e1e4e8; border-radius:6px; font-family:Arial,Helvetica,sans-serif; font-size:14px; color:#1a1a1a;">

<tr><td style="padding:20px 24px 8px 24px; font-size:15px; line-height:1.5;">{{.Lead}}</td></tr>

{{if .Info.AuditName}}<tr><td style="padding:0 24px 8px 24px; color:#57606a; font-size:13px;">Audit: {{.Info.AuditName}}</td></tr>{{end}}

{{if .Info.Actor}}<tr><td style="padding:0 24px 8px 24px; font-size:13px;"><span style="color:#57606a;">{{.ActorLabel}}</span> {{.Info.Actor}}</td></tr>{{end}}

{{if .Info.Items}}<tr><td style="padding:8px 24px 4px 24px;">
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="table-layout:fixed; font-size:13px; border-collapse:collapse;">
<tr style="color:#57606a; text-align:left;">
<td width="29%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8; white-space:nowrap;">Requirement Type</td>
<td width="14%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8; white-space:nowrap;">Control No</td>
<td width="{{if .Info.ShowStatus}}23{{else}}42{{end}}%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8;">Description</td>
<td width="15%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8; white-space:nowrap;">Due Date</td>
{{if .Info.ShowStatus}}<td width="19%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8; white-space:nowrap;">Status</td>{{end}}
</tr>
{{range .Info.Items}}<tr>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; word-break:break-word;">{{.RequirementType}}</td>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; font-weight:bold; word-break:break-word;">{{.ControlNumber}}</td>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; word-break:break-word; overflow-wrap:break-word;">{{.Description}}</td>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; white-space:nowrap;">{{.DueDate}}</td>
{{if $.Info.ShowStatus}}<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; white-space:nowrap;">{{.Tier}}</td>{{end}}
</tr>{{end}}
</table>
</td></tr>{{end}}

{{if .Info.Comment}}<tr><td style="padding:12px 24px 4px 24px;">
<table width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="padding:12px 14px; background-color:#fff8e1; border-left:3px solid #f0ad4e; font-size:14px; line-height:1.5;">
<span style="color:#57606a;">Comment</span><br>{{.Info.Comment}}
</td></tr>
</table>
</td></tr>{{end}}

<tr><td style="padding:20px 24px 24px 24px;">
<a href="{{.Info.DetailURL}}" style="display:inline-block; padding:10px 20px; background-color:#ff7300; color:#ffffff; text-decoration:none; border-radius:4px; font-weight:bold; font-size:14px;">View in Audit Hub</a>
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>`))

// SendAuditEvent notifies one recipient about ev. Unlike SendRiskEvent (many
// recipients, identical content), every audit event already has exactly one
// resolved owner by the time it reaches this call — the batching across that
// owner's items happens in the caller (handler/notify.go, the reminder job),
// not here.
//
// Retries once on a transport failure to ride out a cold start; see
// sendAttempts. Callers must expect this to block for up to two full client
// timeouts and so should not run it on a request path.
func (c *Client) SendAuditEvent(ctx context.Context, ev AuditEvent, to string, info AuditEventInfo) error {
	tpl, ok := auditEventTemplates[ev]
	if !ok {
		return fmt.Errorf("emailer: no template for event %q", ev)
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("emailer: no recipient for event %q", ev)
	}

	var body bytes.Buffer
	if err := auditBodyTemplate.Execute(&body, struct {
		Lead       string
		ActorLabel string
		Info       AuditEventInfo
	}{tpl.lead, tpl.actorLabel, info}); err != nil {
		return fmt.Errorf("emailer: render template: %w", err)
	}

	reqBody := sendEmailRequest{
		To:       []string{to},
		From:     c.from,
		Subject:  sanitizeSubject(tpl.subject(info)),
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
		slog.Warn("emailer: audit send failed, retrying",
			"attempt", attempt, "of", sendAttempts, "err", err)
	}
	return lastErr
}

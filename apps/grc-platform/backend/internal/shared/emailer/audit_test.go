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
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestEveryAuditEventHasATemplate guards the map SendAuditEvent looks up:
// adding an AuditEvent constant without a template would otherwise fail only
// at runtime, as a notification that silently never sends.
func TestEveryAuditEventHasATemplate(t *testing.T) {
	all := []AuditEvent{
		AuditEventOwnerAssigned, AuditEventAuditorAssigned, AuditEventReminderDue10,
		AuditEventReminderDue5, AuditEventReminderOverdue, AuditEventReminderOverdueAdmin,
		AuditEventReminderOverdueLead, AuditEventResubmissionNeeded,
		AuditEventSampleSubmitted, AuditEventEvidenceInternalReview, AuditEventPopulationInternalReview,
		AuditEventEvidenceUnderValidation, AuditEventPopulationUnderValidation,
		AuditEventPopulationCompleteSampleNeeded, AuditEventControlComplete, AuditEventCommentAdded,
	}
	for _, ev := range all {
		tpl, ok := auditEventTemplates[ev]
		if !ok {
			t.Errorf("event %q has no template", ev)
			continue
		}
		if tpl.lead == "" {
			t.Errorf("event %q has an empty lead sentence", ev)
		}
		if got := tpl.subject(AuditEventInfo{Items: []AuditEventItem{{ControlNumber: "C-1"}}}); got == "" {
			t.Errorf("event %q produced an empty subject", ev)
		}
	}
}

func TestSendAuditEventUnknownEventDoesNotSend(t *testing.T) {
	var called bool
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { called = true })

	if err := c.SendAuditEvent(context.Background(), AuditEvent("NOPE"), "a@b.com", AuditEventInfo{}); err == nil {
		t.Fatal("want an error for an unknown event, got nil")
	}
	if called {
		t.Error("an unknown event must not reach the email-service")
	}
}

func TestSendAuditEventBlankRecipientDoesNotSend(t *testing.T) {
	var called bool
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { called = true })

	if err := c.SendAuditEvent(context.Background(), AuditEventOwnerAssigned, "   ", AuditEventInfo{}); err == nil {
		t.Fatal("want an error for a blank recipient, got nil")
	}
	if called {
		t.Error("must not call the email-service with a blank recipient")
	}
}

func TestSendAuditEventSendsToSingleRecipient(t *testing.T) {
	var got []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			To []string `json:"to"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = body.To
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})

	err := c.SendAuditEvent(context.Background(), AuditEventOwnerAssigned, "owner@example.com", AuditEventInfo{
		Items: []AuditEventItem{{ControlNumber: "C-1", RequirementType: "Evidence Requirement"}},
	})
	if err != nil {
		t.Fatalf("SendAuditEvent: %v", err)
	}
	if len(got) != 1 || got[0] != "owner@example.com" {
		t.Errorf("To = %v, want exactly [owner@example.com]", got)
	}
}

// The body is user-supplied in places (item descriptions, rejection
// comments), so it must be HTML-escaped — html/template, not text/template.
func TestSendAuditEventEscapesUserSuppliedText(t *testing.T) {
	var decoded string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Template string `json:"template"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := base64.StdEncoding.DecodeString(body.Template)
		decoded = string(raw)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})

	err := c.SendAuditEvent(context.Background(), AuditEventResubmissionNeeded, "owner@example.com", AuditEventInfo{
		Comment: "<script>alert(1)</script>",
		Items:   []AuditEventItem{{ControlNumber: "C-1", Description: "<b>desc</b>"}},
	})
	if err != nil {
		t.Fatalf("SendAuditEvent: %v", err)
	}
	if strings.Contains(decoded, "<script>") || strings.Contains(decoded, "<b>desc</b>") {
		t.Errorf("rendered body contains unescaped user input:\n%s", decoded)
	}
}

// A mixed control+population batch must still resolve to one subject, not
// fail or render blank.
func TestControlThreadSubjectCoversMixedKinds(t *testing.T) {
	tests := []struct {
		name  string
		items []AuditEventItem
	}{
		{"no items", nil},
		{"one control", []AuditEventItem{{ControlNumber: "C-1", RequirementType: "Evidence Requirement"}}},
		{"one population", []AuditEventItem{{ControlNumber: "C-1", RequirementType: "Population Requirement"}}},
		{"mixed", []AuditEventItem{
			{ControlNumber: "C-1", RequirementType: "Evidence Requirement"},
			{ControlNumber: "C-1", RequirementType: "Population Requirement"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := controlThreadSubject(AuditEventInfo{AuditName: "Q3 Audit", Items: tt.items})
			if got == "" {
				t.Error("subject must never be empty")
			}
		})
	}
}

// The overdue escalation must NOT thread with the control's ordinary workflow
// mail (it re-sends daily and would bury itself), and must stay identical
// across days — and across which/how many controls are overdue that day — so
// each day's digest threads with the previous ones.
func TestOverdueAdminSubjectIsItsOwnStableThread(t *testing.T) {
	info := AuditEventInfo{
		AuditName: "Q3 Audit",
		Items: []AuditEventItem{
			{ControlNumber: "C-1", RequirementType: "Evidence Requirement"},
			{ControlNumber: "C-2", RequirementType: "Population Requirement"},
		},
	}
	got := overdueAdminSubject(info)
	if got == "" {
		t.Fatal("subject must never be empty")
	}
	if got == controlThreadSubject(info) {
		t.Errorf("overdue subject %q must differ from the control thread subject", got)
	}
	if !strings.Contains(got, "Q3 Audit") {
		t.Errorf("subject %q should name the audit", got)
	}
	// The item set changes day to day (which controls are still overdue); the
	// subject must not depend on it.
	info.Items = info.Items[:1]
	if again := overdueAdminSubject(info); again != got {
		t.Errorf("subject changed with the item set (%q vs %q) — daily digests would stop threading", again, got)
	}
	if overdueAdminSubject(AuditEventInfo{AuditName: "Q3 Audit"}) == "" {
		t.Error("subject must never be empty, even with no items")
	}
}

// The lead digest is about one person and spans audits, so its subject names
// the owner rather than an audit — and stays stable as their overdue set
// changes, so each day's escalation threads with the last.
func TestOverdueLeadSubjectNamesTheOwnerAndStaysStable(t *testing.T) {
	info := AuditEventInfo{
		OwnerName: "Jane Doe",
		Items: []AuditEventItem{
			{ControlNumber: "C-1", Audit: "Q3 Audit"},
			{ControlNumber: "C-2", Audit: "Q4 Audit"},
		},
	}
	got := overdueLeadSubject(info)
	if !strings.Contains(got, "Jane Doe") {
		t.Errorf("subject %q should name the owner", got)
	}
	info.Items = info.Items[:1]
	if again := overdueLeadSubject(info); again != got {
		t.Errorf("subject changed with the item set (%q vs %q) — daily digests would stop threading", again, got)
	}
}

// TestEveryAuditEventHasATemplate calls every subject with an info carrying no
// owner name; this is the fallback that keeps it non-empty.
func TestOverdueLeadSubjectFallsBackWithoutAnOwnerName(t *testing.T) {
	if got := overdueLeadSubject(AuditEventInfo{}); got == "" {
		t.Error("subject must never be empty, even with no owner name")
	}
}

// A lead holds no audit privileges, so the digest must carry no links at all —
// including the footer button, which renders unconditionally for every other
// event.
func TestLeadDigestRendersNoLinks(t *testing.T) {
	var decoded string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Template string `json:"template"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := base64.StdEncoding.DecodeString(body.Template)
		decoded = string(raw)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})

	err := c.SendAuditEvent(context.Background(), AuditEventReminderOverdueLead, "lead@example.com", AuditEventInfo{
		OwnerName: "Jane Doe",
		Actor:     "Jane Doe (jane@example.com)",
		ShowAudit: true,
		Items:     []AuditEventItem{{ControlNumber: "C-1", Audit: "Q3 Audit", RequirementType: "Evidence Requirement"}},
	})
	if err != nil {
		t.Fatalf("SendAuditEvent: %v", err)
	}
	if strings.Contains(decoded, "<a href") {
		t.Errorf("lead digest must contain no links:\n%s", decoded)
	}
	if strings.Contains(decoded, "View in Audit Hub") {
		t.Error("lead digest must omit the footer button")
	}
	// ShowAudit is the only place the audit name can appear for this event.
	if !strings.Contains(decoded, "Q3 Audit") {
		t.Errorf("lead digest should label each row's audit:\n%s", decoded)
	}
}

// Every other event sets DetailURL, so making the footer conditional must not
// remove the button from any of them.
func TestFooterButtonStillRendersWhenDetailURLIsSet(t *testing.T) {
	var decoded string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Template string `json:"template"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := base64.StdEncoding.DecodeString(body.Template)
		decoded = string(raw)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})

	err := c.SendAuditEvent(context.Background(), AuditEventOwnerAssigned, "owner@example.com", AuditEventInfo{
		DetailURL: "https://grc.example.com/audit/1/controls/2",
		Items:     []AuditEventItem{{ControlNumber: "C-1"}},
	})
	if err != nil {
		t.Fatalf("SendAuditEvent: %v", err)
	}
	if !strings.Contains(decoded, "View in Audit Hub") {
		t.Error("footer button must still render when DetailURL is set")
	}
}

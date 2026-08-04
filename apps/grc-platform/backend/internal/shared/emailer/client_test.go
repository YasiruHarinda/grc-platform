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
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

// newTestClient wires a Client at a test server that serves both the token
// endpoint and /send-email, so no real credentials or network are involved.
// sendHandler receives every /send-email request.
func newTestClient(t *testing.T, sendHandler http.HandlerFunc) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600}`))
	})
	mux.HandleFunc("/send-email", sendHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return New(srv.URL, "noreply@example.com", srv.URL+"/token", "id", "secret")
}

// dropConnection simulates a transport failure — the request never produces a
// response at all, which is the shape an email-service cold start takes when it
// outlasts the client timeout.
func dropConnection(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	hj, ok := w.(http.Hijacker)
	if !ok {
		t.Error("test server response writer is not a Hijacker")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		t.Errorf("hijack: %v", err)
		return
	}
	_ = conn.Close()
}

// A cold start shows up as a transport failure on the first attempt; the retry
// then lands on the now-warm instance and the send succeeds.
func TestSendRiskCreatedRetriesTransportFailure(t *testing.T) {
	// Dropping the first connection forces the retry onto a new one, which the
	// test server handles on a different goroutine — hence the atomic counter.
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			dropConnection(t, w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Email sent successfully"}`))
	})

	if err := c.SendRiskCreated(context.Background(), "owner@example.com", RiskCreated{RiskCode: "R-1"}); err != nil {
		t.Fatalf("SendRiskCreated: want success after retry, got %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

// Every attempt failing at the transport layer still gives up after
// sendAttempts and surfaces the last error rather than retrying forever.
func TestSendRiskCreatedGivesUpAfterSendAttempts(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		dropConnection(t, w)
	})

	err := c.SendRiskCreated(context.Background(), "owner@example.com", RiskCreated{RiskCode: "R-1"})
	if err == nil {
		t.Fatal("SendRiskCreated: want an error, got nil")
	}
	if got := attempts.Load(); got != sendAttempts {
		t.Errorf("attempts = %d, want %d", got, sendAttempts)
	}
}

// A response the service actually returned is deterministic — retrying it would
// only double the log noise — so it must fail on the first attempt.
func TestSendRiskCreatedDoesNotRetryServiceError(t *testing.T) {
	var attempts atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"to cannot be empty"}`))
	})

	err := c.SendRiskCreated(context.Background(), "owner@example.com", RiskCreated{RiskCode: "R-1"})
	if err == nil {
		t.Fatal("SendRiskCreated: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "to cannot be empty") {
		t.Errorf("error %q does not carry the service's message", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (service errors are not retried)", got)
	}
}

// TestEveryRiskEventHasATemplate guards the map SendRiskEvent looks up: adding
// a RiskEvent constant without a template would otherwise fail only at runtime,
// as a notification that silently never sends.
func TestEveryRiskEventHasATemplate(t *testing.T) {
	all := []RiskEvent{
		EventCreated, EventPendingMgmtApproval, EventComplianceApproved,
		EventActionPlanCompleted, EventPendingOwnerClosure, EventPendingMgmtClosure,
		EventRejected, EventEscalated, EventEscalationCommented,
	}
	for _, ev := range all {
		tpl, ok := eventTemplates[ev]
		if !ok {
			t.Errorf("event %q has no template", ev)
			continue
		}
		if tpl.lead == "" {
			t.Errorf("event %q has an empty lead sentence", ev)
		}
		if got := tpl.subject(RiskEventInfo{RiskCode: "R-1", RiskTitle: "T"}); got == "" {
			t.Errorf("event %q produced an empty subject", ev)
		}
	}
}

func TestSendRiskEventUnknownEventDoesNotSend(t *testing.T) {
	var called bool
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { called = true })

	if err := c.SendRiskEvent(context.Background(), RiskEvent("NOPE"), []string{"a@b.com"}, RiskEventInfo{}); err == nil {
		t.Fatal("want an error for an unknown event, got nil")
	}
	if called {
		t.Error("an unknown event must not reach the email-service")
	}
}

// A blank recipient would draw a 400 the retry logic deliberately won't repeat,
// so they are filtered before the request is built.
func TestSendRiskEventFiltersBlankRecipients(t *testing.T) {
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

	err := c.SendRiskEvent(context.Background(), EventRejected,
		[]string{"a@b.com", "", "   ", "c@d.com"}, RiskEventInfo{RiskCode: "R-1"})
	if err != nil {
		t.Fatalf("SendRiskEvent: %v", err)
	}
	if len(got) != 2 || got[0] != "a@b.com" || got[1] != "c@d.com" {
		t.Errorf("To = %v, want the two non-blank addresses in one message", got)
	}
}

func TestSendRiskEventNoRecipientsDoesNotSend(t *testing.T) {
	var called bool
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { called = true })

	if err := c.SendRiskEvent(context.Background(), EventRejected, []string{"", " "}, RiskEventInfo{}); err == nil {
		t.Fatal("want an error when every recipient is blank, got nil")
	}
	if called {
		t.Error("must not call the email-service with no recipients")
	}
}

// The body is user-supplied in places (title, rejection comment), so it must be
// HTML-escaped — html/template, not text/template.
func TestSendRiskEventEscapesUserSuppliedText(t *testing.T) {
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

	err := c.SendRiskEvent(context.Background(), EventRejected, []string{"a@b.com"},
		RiskEventInfo{RiskCode: "R-1", Comment: "<script>alert(1)</script>"})
	if err != nil {
		t.Fatalf("SendRiskEvent: %v", err)
	}
	if strings.Contains(decoded, "<script>") {
		t.Error("rejection comment was not HTML-escaped")
	}
}

// Unlike the body, the subject is built by plain fmt.Sprintf, not
// html/template — a CR/LF embedded in a free-text field like RiskTitle must
// still not reach the outgoing request unsanitized.
func TestSendRiskEventSanitizesSubjectNewlines(t *testing.T) {
	var subject string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Subject string `json:"subject"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		subject = body.Subject
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})

	err := c.SendRiskEvent(context.Background(), EventCreated, []string{"a@b.com"},
		RiskEventInfo{RiskCode: "R-1", RiskTitle: "Injected\r\nX-Mailer: evil"})
	if err != nil {
		t.Fatalf("SendRiskEvent: %v", err)
	}
	if strings.ContainsAny(subject, "\r\n") {
		t.Errorf("subject still contains a raw CR/LF: %q", subject)
	}
}

func TestSanitizeSubjectCollapsesNewlines(t *testing.T) {
	got := sanitizeSubject("line one\r\nline two\nline three")
	want := "line one line two line three"
	if got != want {
		t.Errorf("sanitizeSubject(...) = %q, want %q", got, want)
	}
}

func TestSanitizeSubjectTruncatesOnRuneBoundary(t *testing.T) {
	// A multi-byte rune ('é', 2 bytes in UTF-8) placed right at the cutoff:
	// byte-slicing would split it and produce invalid UTF-8.
	s := strings.Repeat("a", maxSubjectRunes-1) + "é" + strings.Repeat("b", 10)
	got := sanitizeSubject(s)
	gotRunes := []rune(got)
	if len(gotRunes) != maxSubjectRunes {
		t.Fatalf("sanitizeSubject(...) has %d runes, want %d", len(gotRunes), maxSubjectRunes)
	}
	if !utf8.ValidString(got) {
		t.Errorf("sanitizeSubject(...) produced invalid UTF-8: %q", got)
	}
	if gotRunes[maxSubjectRunes-1] != 'é' {
		t.Errorf("truncation cut the boundary rune instead of stopping after it: %q", got)
	}
}

// Every event must label its Actor for itself: a generic label leaves the
// reader guessing which of the several people on a risk the address belongs to.
func TestEveryRiskEventLabelsItsActor(t *testing.T) {
	for ev, tpl := range eventTemplates {
		if tpl.actorLabel == "" {
			t.Errorf("event %q has no actorLabel", ev)
		}
	}
}

func TestSendRiskEventRendersTheEventsActorLabel(t *testing.T) {
	cases := []struct {
		ev        RiskEvent
		wantLabel string
	}{
		{EventCreated, "Created by"},
		{EventRejected, "Rejected by"},
		{EventActionPlanCompleted, "Completed by"},
	}
	for _, c := range cases {
		t.Run(string(c.ev), func(t *testing.T) {
			var decoded string
			cl := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Template string `json:"template"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				raw, _ := base64.StdEncoding.DecodeString(body.Template)
				decoded = string(raw)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"message":"ok"}`))
			})
			err := cl.SendRiskEvent(context.Background(), c.ev, []string{"a@b.com"},
				RiskEventInfo{RiskCode: "R-1", Actor: "someone@wso2.com"})
			if err != nil {
				t.Fatalf("SendRiskEvent: %v", err)
			}
			// No trailing colon: the label is its own table column now, so the
			// separator is the layout rather than punctuation.
			if !strings.Contains(decoded, c.wantLabel) {
				t.Errorf("body missing %q label", c.wantLabel)
			}
			if strings.Contains(decoded, "Actioned by") {
				t.Error("body still uses the old generic label")
			}
		})
	}
}

// Actor is empty for system-triggered events; the row must disappear rather
// than render a dangling label with no value.
func TestSendRiskEventOmitsActorRowWhenUnset(t *testing.T) {
	var decoded string
	cl := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Template string `json:"template"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := base64.StdEncoding.DecodeString(body.Template)
		decoded = string(raw)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})
	if err := cl.SendRiskEvent(context.Background(), EventEscalated, []string{"a@b.com"},
		RiskEventInfo{RiskCode: "R-1"}); err != nil {
		t.Fatalf("SendRiskEvent: %v", err)
	}
	if strings.Contains(decoded, "Escalated by") {
		t.Error("actor row rendered despite Actor being empty")
	}
}

func renderBody(t *testing.T, ev RiskEvent, info RiskEventInfo) string {
	t.Helper()
	var decoded string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Template string `json:"template"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		raw, _ := base64.StdEncoding.DecodeString(b.Template)
		decoded = string(raw)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})
	if err := c.SendRiskEvent(context.Background(), ev, []string{"a@b.com"}, info); err != nil {
		t.Fatalf("SendRiskEvent: %v", err)
	}
	return decoded
}

// One shared message serves every recipient, so it has to spell out who does
// what — a recipient can't tell from the To header which role is theirs.
func TestSendRiskEventRendersWhoNeedsToAct(t *testing.T) {
	body := renderBody(t, EventComplianceApproved, RiskEventInfo{
		RiskCode: "R-1",
		People: map[string][]string{
			RoleActionOwner:  {"Asel (asel@wso2.com)"},
			RoleRiskAssigner: {"Wethmi (wethmi@wso2.com)"},
		},
	})
	for _, want := range []string{
		"Who needs to act", RoleActionOwner, "Asel (asel@wso2.com)",
		"Complete the action plan steps", RoleRiskAssigner, "Wethmi (wethmi@wso2.com)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

// A risk may have several plans with different owners; every one must appear.
func TestSendRiskEventRendersEveryActionOwner(t *testing.T) {
	body := renderBody(t, EventComplianceApproved, RiskEventInfo{
		RiskCode: "R-1",
		People:   map[string][]string{RoleActionOwner: {"Asel (a@x.com)", "Dineth (d@x.com)"}},
	})
	for _, want := range []string{"Asel (a@x.com)", "Dineth (d@x.com)"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

// A role nobody fills must vanish, not render as a labelled empty row — a risk
// can legitimately have no action owner yet.
func TestSendRiskEventOmitsUnfilledRoles(t *testing.T) {
	body := renderBody(t, EventComplianceApproved, RiskEventInfo{
		RiskCode: "R-1",
		People:   map[string][]string{RoleRiskAssigner: {"Wethmi (w@x.com)"}},
	})
	if strings.Contains(body, RoleActionOwner) {
		t.Error("unfilled Action Owner role was rendered")
	}
	if !strings.Contains(body, RoleRiskAssigner) {
		t.Error("filled Risk Assigner role was dropped")
	}
}

func TestSendRiskEventOmitsBlockWhenNobodyIsNamed(t *testing.T) {
	body := renderBody(t, EventComplianceApproved, RiskEventInfo{RiskCode: "R-1"})
	if strings.Contains(body, "Who needs to act") {
		t.Error("block rendered with no people supplied")
	}
}

// ActionRoles drives which people the caller bothers to resolve, so it must
// match what each template will actually render.
func TestActionRolesMatchesTemplates(t *testing.T) {
	for ev, tpl := range eventTemplates {
		got := ActionRoles(ev)
		if len(got) != len(tpl.actions) {
			t.Errorf("%s: ActionRoles returned %d roles, template declares %d", ev, len(got), len(tpl.actions))
			continue
		}
		for i, a := range tpl.actions {
			if got[i] != a.role {
				t.Errorf("%s: role %d = %q, want %q", ev, i, got[i], a.role)
			}
			if a.instruction == "" {
				t.Errorf("%s: role %q has no instruction", ev, a.role)
			}
		}
	}
	if ActionRoles(RiskEvent("NOPE")) != nil {
		t.Error("unknown event should return no roles")
	}
}

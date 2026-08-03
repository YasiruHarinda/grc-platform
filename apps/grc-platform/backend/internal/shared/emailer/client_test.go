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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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

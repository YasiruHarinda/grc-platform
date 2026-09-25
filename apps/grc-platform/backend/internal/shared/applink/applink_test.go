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

package applink

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
)

var testLinks = New("https://grc.example.com", "https://one.example.com")

func TestForPicksHostByUserType(t *testing.T) {
	tests := []struct {
		userType string
		want     string
	}{
		{"INTERNAL", "https://one.example.com/security/audit/audits/7?control=3"},
		{"EXTERNAL", "https://grc.example.com/audit/audits/7?control=3"},
		{"", "https://grc.example.com/audit/audits/7?control=3"},
		{"CONTRACTOR", "https://grc.example.com/audit/audits/7?control=3"},
	}
	for _, tt := range tests {
		if got := testLinks.For(tt.userType).URL(ControlPath(7, 3)); got != tt.want {
			t.Errorf("For(%q).URL = %q, want %q", tt.userType, got, tt.want)
		}
	}
}

func TestURLLeavesEmptyAndAbsoluteAlone(t *testing.T) {
	r := testLinks.For("INTERNAL")
	if got := r.URL(""); got != "" {
		t.Errorf("URL(\"\") = %q, want empty", got)
	}
	if got := r.URL("https://elsewhere.example.com/x"); got != "https://elsewhere.example.com/x" {
		t.Errorf("absolute URL rewritten: %q", got)
	}
}

func TestResolveInfoCoversFooterItemsAndGroups(t *testing.T) {
	info := emailer.AuditEventInfo{
		DetailURL: DashboardPath,
		Items:     []emailer.AuditEventItem{{DetailURL: AuditPath(1)}, {}},
		Groups: []emailer.AuditEventGroup{
			{Items: []emailer.AuditEventItem{{DetailURL: ControlPath(1, 2)}}},
		},
	}
	got := testLinks.ResolveInfo("INTERNAL", info)
	if got.DetailURL != "https://one.example.com/security/audit/dashboard" {
		t.Errorf("footer = %q", got.DetailURL)
	}
	if got.Items[0].DetailURL != "https://one.example.com/security/audit/audits/1" || got.Items[1].DetailURL != "" {
		t.Errorf("items = %+v", got.Items)
	}
	if got.Groups[0].Items[0].DetailURL != "https://one.example.com/security/audit/audits/1?control=2" {
		t.Errorf("group items = %+v", got.Groups[0].Items)
	}
	if info.DetailURL != DashboardPath || info.Items[0].DetailURL != AuditPath(1) || info.Groups[0].Items[0].DetailURL != ControlPath(1, 2) {
		t.Error("ResolveInfo mutated the caller's info")
	}
}

func TestResolveInfoExternal(t *testing.T) {
	got := testLinks.ResolveInfo("EXTERNAL", emailer.AuditEventInfo{DetailURL: DashboardPath})
	if got.DetailURL != "https://grc.example.com/audit/dashboard" {
		t.Errorf("footer = %q", got.DetailURL)
	}
}

func TestUnrecognisedUserTypeLogsAWarning(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	testLinks.For("CONTRACTOR")
	if out := buf.String(); !strings.Contains(out, "level=WARN") || !strings.Contains(out, "CONTRACTOR") {
		t.Errorf("no warning naming the user type, got log %q", out)
	}

	buf.Reset()
	testLinks.For("INTERNAL")
	testLinks.For("EXTERNAL")
	if buf.Len() != 0 {
		t.Errorf("recognised user types logged %q, want silence", buf.String())
	}
}

func TestResolveInfoNilSlicesStayNil(t *testing.T) {
	got := testLinks.ResolveInfo("INTERNAL", emailer.AuditEventInfo{})
	if got.Items != nil || got.Groups != nil {
		t.Errorf("nil Items/Groups became %v / %v", got.Items, got.Groups)
	}
}

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

// Package applink builds the links inside audit emails. Audit pages live at two
// hosts: internal users reach them through One WSO2 (under /security), external
// auditors stay on the grc-platform webapp. Callers therefore build an
// app-relative path (AuditPath, ControlPath, ...) and the host is chosen per
// recipient, at send time, once the recipient's user type is known.
package applink

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/emailer"
)

const (
	userTypeInternal = "INTERNAL"
	userTypeExternal = "EXTERNAL"

	// OneWSO2SecurityPath is where One WSO2 mounts the GRC pages. It stays in
	// code so ONE_WSO2_WEBAPP_URL remains a bare origin like FRONTEND_BASE_URL.
	OneWSO2SecurityPath = "/security"

	// DashboardPath is the audit dashboard.
	DashboardPath = "/audit/dashboard"
)

// AuditPath is the audit detail page.
func AuditPath(auditID int) string {
	return fmt.Sprintf("/audit/audits/%d", auditID)
}

// ControlPath deep-links to one control's drawer via the ?control= query param
// the audit detail page reads on load.
func ControlPath(auditID, controlID int) string {
	return fmt.Sprintf("/audit/audits/%d?control=%d", auditID, controlID)
}

// Links holds the two origins a link can be built on. Both are bare origins
// (no path, no trailing slash), as config validates them.
type Links struct {
	grcOrigin string // FRONTEND_BASE_URL — grc-platform webapp, external recipients
	oneOrigin string // ONE_WSO2_WEBAPP_URL — One WSO2, internal recipients
}

// New returns Links for the grc-platform webapp origin and the One WSO2 origin.
func New(grcOrigin, oneWSO2Origin string) Links {
	return Links{grcOrigin: grcOrigin, oneOrigin: oneWSO2Origin}
}

// Resolver turns app-relative paths into absolute URLs for one recipient.
type Resolver struct {
	prefix string // origin, plus the One WSO2 mount path where it applies
}

// URL makes path absolute. An empty path stays empty (the email template omits
// a link with no URL), and anything not app-relative is left untouched.
func (r Resolver) URL(path string) string {
	if path == "" || !strings.HasPrefix(path, "/") {
		return path
	}
	return r.prefix + path
}

// For returns the Resolver for a recipient of userType. INTERNAL goes to One
// WSO2, EXTERNAL to grc-platform. Anything else is unexpected — user.user_type
// is NOT NULL DEFAULT 'INTERNAL' — and falls back to grc-platform, the
// pre-existing link target, with a warning rather than dropping the email.
func (l Links) For(userType string) Resolver {
	switch userType {
	case userTypeInternal:
		return Resolver{prefix: l.oneOrigin + OneWSO2SecurityPath}
	case userTypeExternal:
		return Resolver{prefix: l.grcOrigin}
	}
	slog.Warn("audit email link: unrecognised recipient user type, linking to grc-platform", "userType", userType)
	return Resolver{prefix: l.grcOrigin}
}

// ResolveInfo returns info with every DetailURL — the footer button, each item
// row, and each departure group's rows — made absolute for a recipient of
// userType. The caller's info is not modified.
func (l Links) ResolveInfo(userType string, info emailer.AuditEventInfo) emailer.AuditEventInfo {
	r := l.For(userType)
	info.DetailURL = r.URL(info.DetailURL)
	info.Items = resolveItems(r, info.Items)
	if info.Groups != nil {
		groups := make([]emailer.AuditEventGroup, len(info.Groups))
		for i, g := range info.Groups {
			g.Items = resolveItems(r, g.Items)
			groups[i] = g
		}
		info.Groups = groups
	}
	return info
}

func resolveItems(r Resolver, items []emailer.AuditEventItem) []emailer.AuditEventItem {
	if items == nil {
		return nil
	}
	out := make([]emailer.AuditEventItem, len(items))
	for i, it := range items {
		it.DetailURL = r.URL(it.DetailURL)
		out[i] = it
	}
	return out
}

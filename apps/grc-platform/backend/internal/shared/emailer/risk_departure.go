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
	"fmt"
	"html/template"
	"log/slog"
	"strings"
)

// A new shape rather than a new caller: every other risk email is about a single
// risk, so there is no multi-row risk template to extend.

// RiskDepartureItem is one live risk a departed person is still named on.
type RiskDepartureItem struct {
	RiskCode  string
	RiskTitle string
	// Register is the risk's source register.
	Register string
	// Role is what the departed person held on this risk.
	Role string
	// Status is the risk's workflow status, in display form.
	Status string
	// DetailURL links straight to the risk.
	DetailURL string
	// Blocking marks a risk awaiting this person's management approval: the row
	// that sits untouched forever if nobody acts on this email.
	Blocking bool
}

// RiskDepartureGroup is one departed person's block of rows, headed by their
// name and email.
type RiskDepartureGroup struct {
	Person string
	Items  []RiskDepartureItem
}

// RiskDepartureInfo is one recipient's whole digest.
type RiskDepartureInfo struct {
	Groups []RiskDepartureGroup
	// DetailURL backs the footer button: the register list, since this spans risks.
	DetailURL string
}

// Marks a stuck row, so an admin scanning the table sees it without reading
// statuses.
const blockingLabel = "Blocked — awaiting this person"

// Deliberately old-fashioned HTML for the same reason bodyTemplate is: email
// clients drop stylesheets and modern layout.
var riskDepartureBody = template.Must(template.New("riskDeparture").Parse(`<html>
<body style="margin:0; padding:0; background-color:#f4f5f7;">
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#f4f5f7; padding:24px 12px;">
<tr><td align="center">
<table width="900" cellpadding="0" cellspacing="0" border="0" style="width:100%; max-width:900px; background-color:#ffffff; border:1px solid #e1e4e8; border-radius:6px; font-family:Arial,Helvetica,sans-serif; font-size:14px; color:#1a1a1a;">

<tr><td style="padding:20px 24px 8px 24px; font-size:15px; line-height:1.5;">The following people are no longer available in the identity directory. The risk work listed under each of them needs reassigning.</td></tr>

{{range .Groups}}
<tr><td style="padding:16px 24px 0 24px; font-size:14px; font-weight:bold;">{{.Person}}</td></tr>
<tr><td style="padding:8px 24px 4px 24px;">
<table width="100%" cellpadding="0" cellspacing="0" border="0" style="table-layout:fixed; font-size:13px; border-collapse:collapse;">
<tr style="color:#57606a; text-align:left;">
<td width="16%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8; white-space:nowrap;">Risk</td>
<td width="34%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8;">Title</td>
<td width="19%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8;">Role</td>
<td width="31%" style="padding:6px 8px; border-bottom:1px solid #e1e4e8;">Status</td>
</tr>
{{range .Items}}<tr>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; font-weight:bold; word-break:break-word;">{{if .DetailURL}}<a href="{{.DetailURL}}" style="color:#ff7300; text-decoration:none;">{{.RiskCode}}</a>{{else}}{{.RiskCode}}{{end}}<br><span style="font-weight:normal; color:#57606a; font-size:12px;">{{.Register}}</span></td>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; word-break:break-word; overflow-wrap:break-word;">{{.RiskTitle}}</td>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; word-break:break-word;">{{.Role}}</td>
<td style="padding:6px 8px; border-bottom:1px solid #f0f0f0; word-break:break-word;">{{.Status}}{{if .Blocking}}<br><span style="color:#b42318; font-weight:bold;">{{$.BlockingLabel}}</span>{{end}}</td>
</tr>{{end}}
</table>
</td></tr>{{end}}

{{if .Info.DetailURL}}<tr><td style="padding:20px 24px 24px 24px;">
<a href="{{.Info.DetailURL}}" style="display:inline-block; padding:10px 20px; background-color:#ff7300; color:#ffffff; text-decoration:none; border-radius:4px; font-weight:bold; font-size:14px;">View in Risk Hub</a>
</td></tr>{{end}}

</table>
</td></tr>
</table>
</body>
</html>`))

// SendRiskDepartureDigest emails one recipient the newly disabled people who still
// hold live risk work. Retries once, so never call it on a request path.
func (c *Client) SendRiskDepartureDigest(ctx context.Context, to string, info RiskDepartureInfo) error {
	if !c.enabled {
		slog.Info("emailer: notifications disabled, skipping risk departure digest")
		return nil
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("emailer: no recipient for the risk departure digest")
	}
	if len(info.Groups) == 0 {
		return fmt.Errorf("emailer: risk departure digest has nobody in it")
	}

	var body bytes.Buffer
	if err := riskDepartureBody.Execute(&body, struct {
		Info          RiskDepartureInfo
		Groups        []RiskDepartureGroup
		BlockingLabel string
	}{info, info.Groups, blockingLabel}); err != nil {
		return fmt.Errorf("emailer: render template: %w", err)
	}

	return c.deliver(ctx, "risk departure digest", sendEmailRequest{
		To:       []string{to},
		From:     c.from,
		Subject:  sanitizeSubject(DepartureDigestSubject(len(info.Groups))),
		Template: base64.StdEncoding.EncodeToString(body.Bytes()),
	})
}

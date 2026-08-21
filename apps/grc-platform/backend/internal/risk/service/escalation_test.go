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

package service

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// stubUsers resolves one uuid to one id; anything else is "not found".
type stubUsers struct {
	uuid string
	id   int
}

func (s stubUsers) GetByEmail(context.Context, string) (*userentity.User, error) { return nil, nil }
func (s stubUsers) GetByID(context.Context, int) (*userentity.User, error)       { return nil, nil }
func (s stubUsers) GetByUUID(_ context.Context, uuid string) (*userentity.User, error) {
	if uuid != s.uuid {
		return nil, nil
	}
	return &userentity.User{ID: s.id, UUID: uuid}, nil
}
func (s stubUsers) Upsert(context.Context, string, string, string, string) (*userentity.User, error) {
	return nil, nil
}
func (s stubUsers) List(context.Context) ([]*userentity.User, error) { return nil, nil }

func strp(s string) *string { return &s }

// Who may answer an escalation depends on the risk's level: HIGH is the
// Management Approver's call, MEDIUM/LOW belongs to a line manager. Getting
// this wrong either strands a risk nobody can un-escalate, or lets the wrong
// person wave one through.
func TestAuthorizeComment(t *testing.T) {
	const caller = "885aeeb0-2086-4ca4-83c9-b2a62b299967"
	high := &model.RiskDetail{
		ManagementApproverID: 42,
		EffectiveScore:       &model.RiskScore{RiskLevel: "HIGH"},
	}
	medium := &model.RiskDetail{
		ManagementApproverID: 42,
		EffectiveScore:       &model.RiskScore{RiskLevel: "MEDIUM"},
	}

	cases := []struct {
		name        string
		detail      *model.RiskDetail
		esc         *model.Escalation
		users       stubUsers
		canOverride bool
		wantErr     bool
	}{
		{
			"HIGH: the management approver may comment",
			high, &model.Escalation{}, stubUsers{caller, 42}, false, false,
		},
		{
			"HIGH: anyone else may not",
			high, &model.Escalation{}, stubUsers{caller, 7}, false, true,
		},
		{
			"HIGH: a lead is not enough",
			high, &model.Escalation{AssignerLeadUUID: strp(caller)}, stubUsers{caller, 7}, false, true,
		},
		{
			"MEDIUM: the assigner's lead may comment",
			medium, &model.Escalation{AssignerLeadUUID: strp(caller)}, stubUsers{caller, 7}, false, false,
		},
		{
			"MEDIUM: the action owner's lead may comment",
			medium, &model.Escalation{ActionOwnerLeadUUID: strp(caller)}, stubUsers{caller, 7}, false, false,
		},
		{
			// uuid matching is exact, unlike the email matching it replaced —
			// Asgardeo ids come back in one consistent case, so there is no
			// folding to preserve. A near-miss (wrong case, stray space) is
			// correctly a different identity, not the same one spelled
			// differently.
			"MEDIUM: uuid matching is exact, not fuzzy",
			medium, &model.Escalation{AssignerLeadUUID: strp("885AEEB0-2086-4CA4-83C9-B2A62B299967")}, stubUsers{caller, 7}, false, true,
		},
		{
			"MEDIUM: the management approver is not a lead",
			medium, &model.Escalation{}, stubUsers{caller, 42}, false, true,
		},
		{
			"MEDIUM: no leads recorded means nobody but an admin",
			medium, &model.Escalation{}, stubUsers{caller, 7}, false, true,
		},
		// The escape hatch: without it, an escalation whose named commenter has
		// left the company would strand the risk permanently.
		{
			"compliance admin overrides on HIGH",
			high, &model.Escalation{}, stubUsers{caller, 7}, true, false,
		},
		{
			"compliance admin overrides on MEDIUM",
			medium, &model.Escalation{}, stubUsers{caller, 7}, true, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &escalationService{users: c.users}
			err := s.authorizeComment(context.Background(), c.detail, c.esc, caller, c.canOverride)
			if (err != nil) != c.wantErr {
				t.Errorf("authorizeComment err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

// The level drives the comment gate and the escalation email's recipient list,
// so the fallback when a risk has never been reassessed has to be right.
func TestIsHighRisk(t *testing.T) {
	cases := []struct {
		name   string
		detail *model.RiskDetail
		want   bool
	}{
		{"effective HIGH", &model.RiskDetail{EffectiveScore: &model.RiskScore{RiskLevel: "HIGH"}}, true},
		{"effective MEDIUM", &model.RiskDetail{EffectiveScore: &model.RiskScore{RiskLevel: "MEDIUM"}}, false},
		// A reassessment that lowered the level must win — it is what the
		// registers table shows, and the two must not disagree.
		{
			"effective beats gross",
			&model.RiskDetail{
				EffectiveScore: &model.RiskScore{RiskLevel: "LOW"},
				GrossScore:     &model.RiskScore{RiskLevel: "HIGH"},
			},
			false,
		},
		{"falls back to gross when never reassessed", &model.RiskDetail{GrossScore: &model.RiskScore{RiskLevel: "HIGH"}}, true},
		{"no scores at all is not high", &model.RiskDetail{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isHighRisk(c.detail); got != c.want {
				t.Errorf("isHighRisk = %v, want %v", got, c.want)
			}
		})
	}
}

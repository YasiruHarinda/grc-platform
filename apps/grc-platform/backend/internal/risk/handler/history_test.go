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

package handler

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// The history duplicates two routing rules the service owns — where a rejection
// is stamped, and where a resubmit goes back to. If either drifts, the recorded
// history quietly describes something that didn't happen, which is worse than
// no history at all. These pin them to the service's behaviour.

func TestRejectionStageFor(t *testing.T) {
	cases := []struct{ status, want string }{
		{model.StatusPendingOwnerApproval, "OWNER"},
		{model.StatusPendingAmendment, "OWNER"},
		{model.StatusPendingManagementApproval, "MANAGEMENT"},
		{model.StatusPendingComplianceReview, "COMPLIANCE"},
		{model.StatusPendingOwnerCompletion, "COMPLETION_OWNER"},
		{model.StatusPendingManagementClosure, "COMPLETION_MANAGEMENT"},
	}
	for _, c := range cases {
		if got := rejectionStageFor(c.status); got != c.want {
			t.Errorf("rejectionStageFor(%s) = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestResubmitTargetFor(t *testing.T) {
	cases := []struct{ stage, want string }{
		{"COMPLETION_OWNER", model.StatusPendingOwnerCompletion},
		{"COMPLETION_MANAGEMENT", model.StatusPendingManagementClosure},
		{"OWNER", model.StatusPendingOwnerApproval},
		{"MANAGEMENT", model.StatusPendingOwnerApproval},
		{"COMPLIANCE", model.StatusPendingOwnerApproval},
		{"", model.StatusPendingOwnerApproval},
	}
	for _, c := range cases {
		if got := resubmitTargetFor(c.stage); got != c.want {
			t.Errorf("resubmitTargetFor(%q) = %q, want %q", c.stage, got, c.want)
		}
	}
}

// recordOwnerApprove recomputes OwnerApprove's ACCEPT+HIGH routing to name the
// destination. A wrong answer here writes a history entry claiming the risk
// went somewhere it didn't.
func TestRecordOwnerApproveDestination(t *testing.T) {
	strp := func(s string) *string { return &s }
	score := func(l string) *model.RiskScore { return &model.RiskScore{RiskLevel: l} }

	cases := []struct {
		name       string
		from       string
		treatment  *string
		gross      *model.RiskScore
		wantTo     string
		wantRecord bool
	}{
		{"initial, ordinary", model.StatusPendingOwnerApproval, strp("MITIGATE"), score("HIGH"), model.StatusPendingComplianceReview, true},
		{"initial, accept+high", model.StatusPendingOwnerApproval, strp("ACCEPT"), score("HIGH"), model.StatusPendingManagementApproval, true},
		{"amendment, accept+high", model.StatusPendingAmendment, strp("ACCEPT"), score("HIGH"), model.StatusPendingManagementApproval, true},
		{"completion, ordinary", model.StatusPendingOwnerCompletion, strp("ACCEPT"), score("MEDIUM"), model.StatusPendingComplianceClosure, true},
		{"completion, accept+high", model.StatusPendingOwnerCompletion, strp("ACCEPT"), score("HIGH"), model.StatusPendingManagementClosure, true},
		// Defensive: a nil treatment or score must not route to management.
		{"nil treatment", model.StatusPendingOwnerApproval, nil, score("HIGH"), model.StatusPendingComplianceReview, true},
		{"nil gross", model.StatusPendingOwnerApproval, strp("ACCEPT"), nil, model.StatusPendingComplianceReview, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got model.HistoryDetails
			rec := &recordingHistory{onRecord: func(r model.RecordHistoryRequest) {
				if r.Details != nil {
					got = *r.Details
				}
			}}
			d := &Deps{History: rec}
			detail := &model.RiskDetail{TreatmentStrategy: c.treatment, GrossScore: c.gross}
			d.recordOwnerApprove(context.Background(), 1, detail, c.from, "by@wso2.com")

			if got.To != c.wantTo {
				t.Errorf("recorded To = %q, want %q", got.To, c.wantTo)
			}
			if got.From != c.from {
				t.Errorf("recorded From = %q, want %q", got.From, c.from)
			}
			if got.Role != "Risk Owner" {
				t.Errorf("recorded Role = %q, want \"Risk Owner\"", got.Role)
			}
		})
	}
}

// recordingHistory captures what the handler writes, without a Compliance Entity.
type recordingHistory struct {
	onRecord func(model.RecordHistoryRequest)
	err      error
}

func (r *recordingHistory) List(context.Context, int) ([]*model.HistoryEntry, error) {
	return nil, nil
}
func (r *recordingHistory) Record(_ context.Context, _ int, req model.RecordHistoryRequest, _ string) error {
	if r.onRecord != nil {
		r.onRecord(req)
	}
	return r.err
}

// A history failure must never surface: the action it records has already been
// committed, so failing here would report an error for work that succeeded.
func TestRecordHistorySwallowsFailure(t *testing.T) {
	d := &Deps{History: &recordingHistory{err: errStub}}
	// Must not panic and must return normally.
	d.recordEvent(context.Background(), 1, "by@wso2.com", model.HistoryApprove, model.HistoryDetails{})
}

// A nil History service (any deployment that hasn't wired it) must be inert
// rather than a nil-pointer panic on every workflow action.
func TestRecordHistoryToleratesNilService(t *testing.T) {
	d := &Deps{}
	d.recordEvent(context.Background(), 1, "by@wso2.com", model.HistoryApprove, model.HistoryDetails{})
}

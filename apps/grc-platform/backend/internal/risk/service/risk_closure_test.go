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
	"testing"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/risk/model"
)

// needsManagementSignOff decides both management stages, so getting it wrong
// either strands non-ACCEPT/HIGH risks at an approval nobody expects, or lets
// an ACCEPT+HIGH risk close without the sign-off it exists to require.
func TestNeedsManagementSignOff(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	score := func(level string) *model.RiskScore { return &model.RiskScore{RiskLevel: level} }

	cases := []struct {
		name      string
		treatment *string
		gross     *model.RiskScore
		want      bool
	}{
		{"accept + high needs sign-off", strPtr("ACCEPT"), score("HIGH"), true},
		{"accept + medium does not", strPtr("ACCEPT"), score("MEDIUM"), false},
		{"accept + low does not", strPtr("ACCEPT"), score("LOW"), false},
		{"mitigate + high does not", strPtr("MITIGATE"), score("HIGH"), false},
		{"transfer + high does not", strPtr("TRANSFER"), score("HIGH"), false},
		// Defensive: a risk with no treatment or no gross score must not be
		// routed to management on a nil dereference or an empty-string match.
		{"nil treatment does not", nil, score("HIGH"), false},
		{"nil gross score does not", strPtr("ACCEPT"), nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			detail := &model.RiskDetail{TreatmentStrategy: c.treatment, GrossScore: c.gross}
			if got := needsManagementSignOff(detail); got != c.want {
				t.Errorf("needsManagementSignOff = %v, want %v", got, c.want)
			}
		})
	}
}

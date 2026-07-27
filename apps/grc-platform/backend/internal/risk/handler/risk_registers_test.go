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

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/middleware"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/privilege"
)

// contextFor builds a context carrying a resolved privilege set the way the
// Auth middleware would, for the given role. Exercises the exact path
// isActionOwnerOnly reads (middleware.UserInfoFromContext + privilege.FromContext)
// without needing a live server or a signed JWT — AUTH_TOKEN_VALIDATOR_ENABLED=false
// skips loading a real privilege.Store entirely (see cmd/server/main.go),
// making this the only way to test privilege-gated behaviour without a real IdP.
func contextFor(t *testing.T, role string) context.Context {
	t.Helper()
	store := privilege.NewForTest(map[string]map[string]bool{
		"grc-platform-risk-action-owner": {
			privilege.ViewRisks:           true,
			privilege.CompleteActionSteps: true,
		},
		"grc-platform-risk-management": {
			privilege.ViewRisks:                  true,
			privilege.ManagementApproveRisk:      true,
			privilege.CreateManagementActionPlan: true,
		},
		"grc-platform-risk-admin": {
			privilege.ViewRisks:           true,
			privilege.CreateRisk:          true,
			privilege.CompleteActionSteps: true,
		},
	})
	ctx := middleware.WithUserInfo(context.Background(), &middleware.UserInfo{Email: "test@wso2.com"})
	return privilege.WithContext(ctx, store.Resolve([]string{role}))
}

func TestIsActionOwnerOnly(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{"grc-platform-risk-action-owner", true},
		{"grc-platform-risk-management", false},
		{"grc-platform-risk-admin", false}, // holds CompleteActionSteps AND CreateRisk
	}
	for _, c := range cases {
		got := isActionOwnerOnly(contextFor(t, c.role))
		if got != c.want {
			t.Errorf("isActionOwnerOnly(%s) = %v, want %v", c.role, got, c.want)
		}
	}
}

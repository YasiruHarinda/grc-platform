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

package directorysync

import (
	"context"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/adminactivity"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// StatusRepository is the one write a run makes. Satisfied by user.Repository.
type StatusRepository interface {
	UpdateStatus(ctx context.Context, id int, status, actor string) (*user.User, error)
}

// ActivityLogger records the write in the admin activity log. Satisfied by
// *adminactivity.Client.
type ActivityLogger interface {
	Log(ctx context.Context, actor, action, entityType string, entityID int, details map[string]any)
}

// Disable writes the Disabled status and logs it under the sync's reserved
// actor. One step rather than two, because a status change the activity log
// cannot account for reads as an unexplained admin action.
func Disable(ctx context.Context, users StatusRepository, activityLog ActivityLogger, u User, name string) error {
	if _, err := users.UpdateStatus(ctx, u.ID, StatusDisabled, adminactivity.ActorDirectoryStatusSync); err != nil {
		return err
	}
	label := name
	if label == "" {
		label = u.UUID
	}
	activityLog.Log(ctx, adminactivity.ActorDirectoryStatusSync, adminactivity.ActionStatusChanged,
		adminactivity.EntityUser, u.ID, map[string]any{"user": label, "status": StatusDisabled})
	return nil
}

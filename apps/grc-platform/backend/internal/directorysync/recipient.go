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
	"fmt"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// Recipient is the slice of a platform user a digest send needs: enough to tell
// they are still a valid target and to resolve their address.
type Recipient struct {
	UUID     string
	UserType string
	Status   string
}

// DeliverableEmail resolves a digest recipient to an email address. Every step
// that fails returns an error rather than an empty string: a hub's digest send
// is gated on reaching someone, so an unresolvable recipient must read as a
// failure and never as a silent skip — otherwise a status gets written against a
// notice that never went out. Both hubs share this so that invariant lives once.
//
// getUser returns the recipient's row, or (nil, nil) when there is none.
// lookupEmail resolves an address from the identity directory, returning ok=false
// when there is none on file; it must not fail, since a directory miss is "no
// address here" rather than a reason to abort the run.
func DeliverableEmail(
	ctx context.Context,
	userID int,
	getUser func(ctx context.Context, id int) (*Recipient, error),
	lookupEmail func(ctx context.Context, uuid, userType string) (string, bool),
) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("invalid recipient id %d", userID)
	}
	u, err := getUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("resolve recipient %d: %w", userID, err)
	}
	if u == nil {
		return "", fmt.Errorf("recipient %d has no user row", userID)
	}
	if u.Status != "" && u.Status != user.StatusActive {
		return "", fmt.Errorf("recipient %d is not active", userID)
	}
	email, ok := lookupEmail(ctx, u.UUID, u.UserType)
	if !ok || email == "" {
		return "", fmt.Errorf("recipient %d has no email on file", userID)
	}
	return email, nil
}

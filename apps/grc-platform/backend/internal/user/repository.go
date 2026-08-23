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

package user

import "context"

// Repository defines the data-access contract for the shared user entity.
// Implementations talk to the Compliance Entity over HTTP — the GRC backend
// never queries the `user` table directly.
//
// GetByEmail and GetByID return (nil, nil) when no such user exists, so callers
// can treat "not found" as a domain condition rather than an error.
// TODO: extend as user-related endpoints are implemented
type Repository interface {
	GetByID(ctx context.Context, id int) (*User, error)
	// GetByUUID resolves a user by their Asgardeo id. Returns (nil, nil) when
	// no user carries that uuid — including a row whose uuid has not been
	// backfilled yet — so a caller distinguishes "nobody" from a lookup failure.
	GetByUUID(ctx context.Context, uuid string) (*User, error)
	// Upsert creates the user if their uuid is unknown, or refreshes it if a
	// row for it already exists. actor is recorded as created_by/updated_by.
	//
	// uuid is required: the `user` table stores no email or display name to
	// fall back on as a matching key (see shared.sql), and the column is
	// NOT NULL. A caller that cannot resolve one (e.g. the identity directory
	// is unreachable) must refuse the provision rather than calling this with
	// an empty string.
	Upsert(ctx context.Context, uuid, actor string) (*User, error)
	// UpsertTyped is Upsert with an explicit user_type (INTERNAL/EXTERNAL) —
	// the Admin Console's Add User flow, which is the only caller that ever
	// provisions an EXTERNAL person. Upsert itself stays INTERNAL-only
	// (its callers never resolve an external identity).
	UpsertTyped(ctx context.Context, uuid, userType, actor string) (*User, error)
	// UpdateStatus sets a user's status (ACTIVE/INACTIVE/REMOVED) — the Admin
	// Console's Users table status control. The entity has supported this on
	// UpdateUserRequest since before this method existed; nothing else in the
	// GRC backend exposed it.
	UpdateStatus(ctx context.Context, id int, status, actor string) (*User, error)
	List(ctx context.Context) ([]*User, error)
}

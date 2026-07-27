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
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id int) (*User, error)
	// Upsert creates the user if their email is unknown, or refreshes their
	// display name if it isn't. actorEmail is recorded as created_by/updated_by.
	Upsert(ctx context.Context, email, displayName, actorEmail string) (*User, error)
	List(ctx context.Context) ([]*User, error)
}

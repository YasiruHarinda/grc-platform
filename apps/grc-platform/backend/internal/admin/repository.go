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

package admin

import "context"

// Repository covers the two entity reads nothing else in the GRC backend
// already exposes: a user list with grants embedded, and the role catalogue.
// Everything else the Admin Console needs (provisioning a user, granting or
// revoking a role, directory search) is already covered by
// internal/user.Repository, internal/shared/grant.Repository and
// internal/directory.Service respectively — see handler/deps.go — so this
// interface is deliberately small rather than a second, overlapping user/grant
// abstraction.
type Repository interface {
	// SearchUsers returns platform users (optionally filtered by a
	// name/email substring) with their active grants embedded.
	SearchUsers(ctx context.Context, query string) ([]User, error)
	// ListRoles returns every RISK- and SHARED-module role — never AUDIT.
	//
	// AUDIT roles are real and already grantable elsewhere in the platform,
	// but are deliberately withheld from this console's picker: Audit Hub
	// still resolves identity by email in several places, and a user
	// provisioned here has no email (see the uuid-identity migration and
	// ADMIN_CONSOLE_DESIGN.md). Granting them an AUDIT role today would look
	// like it worked and then silently misbehave in Audit Hub. Revisit once
	// Audit Hub completes its own uuid migration.
	ListRoles(ctx context.Context) ([]Role, error)
}

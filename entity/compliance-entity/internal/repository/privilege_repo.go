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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// PrivilegeRepository reads the role→privilege mapping.
type PrivilegeRepository interface {
	RolePrivilegeMap(ctx context.Context) (map[string][]string, error)
}

type privilegeRepo struct{ db *sql.DB }

// NewPrivilegeRepository constructs a PrivilegeRepository.
func NewPrivilegeRepository(db *sql.DB) PrivilegeRepository { return &privilegeRepo{db: db} }

// RolePrivilegeMap returns every active grant, keyed by role name. A grant is
// active only when the join row, the role and the privilege are all active, so
// deactivating any one of the three revokes it.
func (r *privilegeRepo) RolePrivilegeMap(ctx context.Context) (map[string][]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.role_name, p.privilege_name
		FROM role_privilege rp
		JOIN role r ON r.id = rp.role_id
		JOIN privilege p ON p.id = rp.privilege_id
		WHERE rp.is_active = TRUE
		  AND r.status = 'ACTIVE'
		  AND p.status = 'ACTIVE'
		ORDER BY r.role_name, p.privilege_name`)
	if err != nil {
		return nil, fmt.Errorf("privilege.RolePrivilegeMap: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]string)
	for rows.Next() {
		var role, priv string
		if err := rows.Scan(&role, &priv); err != nil {
			return nil, fmt.Errorf("privilege.RolePrivilegeMap scan: %w", err)
		}
		out[role] = append(out[role], priv)
	}
	return out, rows.Err()
}

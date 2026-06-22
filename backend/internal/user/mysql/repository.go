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

package mysql

import (
	"context"
	"database/sql"

	"github.com/wso2-open-operations/grc-platform/backend/internal/user"
)

type repository struct{ db *sql.DB }

// NewRepository creates a MySQL-backed user.Repository.
func NewRepository(db *sql.DB) user.Repository {
	return &repository{db: db}
}

func (r *repository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	// TODO: SELECT from user WHERE email = ? AND status != 'REMOVED'
	return nil, nil
}

func (r *repository) GetByID(ctx context.Context, id int) (*user.User, error) {
	// TODO: SELECT from user WHERE id = ? AND status != 'REMOVED'
	return nil, nil
}

func (r *repository) Upsert(ctx context.Context, email, displayName string) (*user.User, error) {
	// TODO: INSERT ... ON DUPLICATE KEY UPDATE display_name = VALUES(display_name)
	return nil, nil
}

func (r *repository) List(ctx context.Context) ([]*user.User, error) {
	// TODO: SELECT from user WHERE status = 'ACTIVE' ORDER BY display_name
	return nil, nil
}

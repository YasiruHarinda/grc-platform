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

// Package entity provides HTTP-client implementations of the risk repository
// interfaces, backed by the Compliance Entity instead of direct MySQL access.
//
// Each type here mirrors one implementation in
// internal/risk/repository/mysql and satisfies the same interface from
// internal/risk/repository, so the two are interchangeable at wiring time.
// buildRiskDeps chooses between them per repository via RISK_ENTITY_REPOS
// while the migration is in progress.
//
// The entity's risk endpoints were written before the risk module existed and
// have never served a request, so where they disagree with the MySQL
// implementation the MySQL implementation is authoritative and the entity is
// corrected to match — never the other way round.
package entity

import (
	"errors"
)

// pageLimit is the entity's maximum page size; List methods page through all
// results by requesting pageLimit at a time until a short page comes back.
const pageLimit = 100

// errNotImplemented mirrors the MySQL package's stub error. Methods that are
// unimplemented there stay unimplemented here — migrating a repository must not
// quietly add behaviour the module did not have.
var errNotImplemented = errors.New("not implemented")

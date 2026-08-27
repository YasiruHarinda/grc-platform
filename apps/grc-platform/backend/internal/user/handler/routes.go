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

// Package handler contains HTTP handlers for genuinely cross-module user
// endpoints. The only one today is GET /api/v1/me/profile, the app-wide
// account menu. The Risk module's own user list / resolve endpoints, which
// used to live here despite being Risk-only, now sit under /api/v1/risk/...
// in internal/risk/handler.
package handler

import (
	"net/http"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/hrentity"
	userentity "github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/user"
)

// Deps holds dependencies for shared user handlers.
type Deps struct {
	Users    userentity.Repository
	HREntity *hrentity.Client
}

// RegisterRoutes mounts the cross-module user routes onto mux.
func RegisterRoutes(mux *http.ServeMux, deps Deps) {
	mux.HandleFunc("GET /api/v1/me/profile", handleGetMyProfile(deps.HREntity, deps.Users))
}

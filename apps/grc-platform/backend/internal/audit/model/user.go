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

// Package model defines the domain types for the Audit Hub module.
package model

// UserRef is a lightweight user record returned by GET /api/v1/audit/users.
// Used to populate owner/auditor dropdowns in the UI.

type UserRef struct {
	ID          int     `json:"id"`
	DisplayName string  `json:"displayName"`
	Email       string  `json:"email"`
	UserType    string  `json:"userType"` // INTERNAL | EXTERNAL
	ProfileURL  *string `json:"profileUrl"`
	// Status is ACTIVE | INACTIVE | REMOVED — used by the notification
	// dispatcher to skip emailing a user who is no longer active.
	Status string `json:"status"`
}

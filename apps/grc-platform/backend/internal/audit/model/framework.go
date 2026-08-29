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

import "time"

// AuditFramework represents a compliance standard (SOC2, ISO 27001, HIPAA, …).
type AuditFramework struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Version   *string   `json:"version"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateFrameworkRequest is the payload for POST /api/v1/audits/frameworks.
type CreateFrameworkRequest struct {
	Name    string  `json:"name"`
	Version *string `json:"version"`
}

// CreateProductRequest is the payload for POST /api/v1/audits/products.
type CreateProductRequest struct {
	Name string `json:"name"`
}

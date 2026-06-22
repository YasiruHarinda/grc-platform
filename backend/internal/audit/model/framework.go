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

// AuditFramework represents a compliance framework (ISO 27001, SOC 2).
// TODO: add fields based on `audit_framework` in audit_schema.sql
type AuditFramework struct{}

// CreateFrameworkRequest is the payload for POST /api/v1/audit/frameworks.
// TODO: define fields (name, description, version)
type CreateFrameworkRequest struct{}

// CreateProductRequest is the payload for POST /api/v1/audit/products.
// TODO: define fields (name, description, owner)
type CreateProductRequest struct{}

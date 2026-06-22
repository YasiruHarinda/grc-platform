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

// Package file provides Azure Blob Storage upload and delete operations used
// by evidence attachments in both the Risk and Audit modules.
package file

import (
	"context"
	"io"
)

// StorageConfig holds Azure Blob Storage credentials from environment variables.
type StorageConfig struct {
	AccountName   string
	AccountKey    string
	ContainerName string
}

// Service wraps Azure Blob Storage operations.
// TODO: implement using the Azure SDK or Shared Key REST API
type Service struct {
	cfg StorageConfig
}

// NewService creates a new file Service.
func NewService(cfg StorageConfig) *Service {
	return &Service{cfg: cfg}
}

// Upload stores the content of r under blobName and returns the blob URL.
// TODO: implement
func (s *Service) Upload(ctx context.Context, blobName string, r io.Reader, contentType string) (string, error) {
	// TODO: PUT request to Azure Blob Storage, return blob URL
	return "", nil
}

// Delete removes the blob with the given name.
// TODO: implement
func (s *Service) Delete(ctx context.Context, blobName string) error {
	// TODO: DELETE request to Azure Blob Storage
	return nil
}

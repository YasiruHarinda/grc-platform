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

// Package entityclient is a small typed HTTP client to the Compliance Entity,
// copied from the backend's internal/shared/entityclient pattern. The MCP
// server uses it for all data access — it never touches MySQL or Azure
// directly (the entity is the only holder of the Azure account key).
package entityclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client to the Compliance Entity base URL.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a client pointed at the Compliance Entity (e.g. http://entity:8080).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// entityError mirrors the entity's error body: {"code":..,"message":".."}.
type entityError struct {
	Message string `json:"message"`
}

// Error is a non-2xx entity response.
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("entity responded %d: %s", e.StatusCode, e.Message)
}

// Get performs GET path and decodes a 2xx JSON body into out (out may be nil).
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// Post performs POST path with a JSON body, decoding the 2xx response into out.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

// GetBytes performs GET path and returns the raw body bytes plus the
// Content-Type and X-File-Name headers (used for evidence file content).
// maxBytes bounds the read; a larger body returns an error.
func (c *Client) GetBytes(ctx context.Context, path string, maxBytes int64) (data []byte, contentType, fileName string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("entityclient: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", "", &Error{StatusCode: http.StatusServiceUnavailable, Message: "data service unavailable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", "", readError(resp)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("entityclient: read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", "", fmt.Errorf("file exceeds the %d byte limit for AI review", maxBytes)
	}
	return data, resp.Header.Get("Content-Type"), resp.Header.Get("X-File-Name"), nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("entityclient: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("entityclient: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{StatusCode: http.StatusServiceUnavailable, Message: "data service unavailable"}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readError(resp)
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("entityclient: decode response: %w", err)
	}
	return nil
}

func readError(resp *http.Response) error {
	msg := "data service error"
	var e entityError
	if raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)); len(raw) > 0 {
		if json.Unmarshal(raw, &e) == nil && e.Message != "" {
			msg = e.Message
		}
	}
	return &Error{StatusCode: resp.StatusCode, Message: msg}
}

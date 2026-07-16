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

// Package config loads environment configuration for the two AI validation
// binaries (agent and MCP server). Mirrors the backend's envOrDefault pattern.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Agent holds configuration for the Validation Agent component (cmd/agent).
type Agent struct {
	Port              string
	LogLevel          string
	AnthropicAPIKey   string
	AnthropicModel    string
	AgentAPIKey       string // inbound bearer expected from the GRC backend
	MCPBaseURL        string
	MCPSharedSecret   string // bootstrap secret for POST /internal/sessions
	ValidationTimeout time.Duration
	MaxLoopIterations int
	MaxFilesPerJob    int
}

// LoadAgent reads the agent configuration from environment variables.
func LoadAgent() (Agent, error) {
	apiKey, err := mustEnv("ANTHROPIC_API_KEY")
	if err != nil {
		return Agent{}, err
	}
	agentKey, err := mustEnv("AGENT_API_KEY")
	if err != nil {
		return Agent{}, err
	}
	secret, err := mustEnv("MCP_SHARED_SECRET")
	if err != nil {
		return Agent{}, err
	}
	return Agent{
		Port:              envOrDefault("PORT", ":8090"),
		LogLevel:          envOrDefault("LOG_LEVEL", "info"),
		AnthropicAPIKey:   apiKey,
		AnthropicModel:    envOrDefault("ANTHROPIC_MODEL", "claude-sonnet-4-6"),
		AgentAPIKey:       agentKey,
		MCPBaseURL:        envOrDefault("MCP_BASE_URL", "http://localhost:8091"),
		MCPSharedSecret:   secret,
		ValidationTimeout: time.Duration(envIntOrDefault("VALIDATION_TIMEOUT_SECONDS", 300)) * time.Second,
		MaxLoopIterations: envIntOrDefault("MAX_LOOP_ITERATIONS", 12),
		MaxFilesPerJob:    envIntOrDefault("MAX_FILES_PER_JOB", 12),
	}, nil
}

// MCPServer holds configuration for the MCP Server component (cmd/mcpserver).
type MCPServer struct {
	Port                    string
	LogLevel                string
	ComplianceEntityBaseURL string
	MCPSharedSecret         string
	SessionTTL              time.Duration
	MaxFileBytesToLLM       int64
}

// LoadMCPServer reads the MCP server configuration from environment variables.
func LoadMCPServer() (MCPServer, error) {
	secret, err := mustEnv("MCP_SHARED_SECRET")
	if err != nil {
		return MCPServer{}, err
	}
	return MCPServer{
		Port:                    envOrDefault("PORT", ":8091"),
		LogLevel:                envOrDefault("LOG_LEVEL", "info"),
		ComplianceEntityBaseURL: envOrDefault("COMPLIANCE_ENTITY_BASE_URL", "http://localhost:8081"),
		MCPSharedSecret:         secret,
		SessionTTL:              time.Duration(envIntOrDefault("SESSION_TTL_SECONDS", 600)) * time.Second,
		MaxFileBytesToLLM:       int64(envIntOrDefault("MAX_FILE_BYTES_TO_LLM", 10<<20)),
	}, nil
}

func mustEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable is not set: %s", key)
	}
	return v, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

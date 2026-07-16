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

package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bridgeTools converts MCP tool definitions into Anthropic tool params,
// restricted to the task's allowlist (so the model is never offered a tool the
// MCP server would reject anyway). Order follows allowed for byte-stable,
// cache-friendly tool blocks.
func bridgeTools(mcpTools []*mcp.Tool, allowed []string) ([]anthropic.ToolUnionParam, error) {
	byName := make(map[string]*mcp.Tool, len(mcpTools))
	for _, t := range mcpTools {
		byName[t.Name] = t
	}
	out := make([]anthropic.ToolUnionParam, 0, len(allowed))
	for _, name := range allowed {
		t, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("bridge: MCP server did not expose required tool %q", name)
		}
		// Client-side InputSchema is a decoded map[string]any; re-marshal to raw
		// JSON so we can pull out properties/required uniformly.
		rawSchema, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("bridge: tool %q schema marshal: %w", name, err)
		}
		schema, err := toInputSchema(rawSchema)
		if err != nil {
			return nil, fmt.Errorf("bridge: tool %q schema: %w", name, err)
		}
		tp := anthropic.ToolParam{
			Name:        t.Name,
			InputSchema: schema,
		}
		if t.Description != "" {
			tp.Description = anthropic.String(t.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return out, nil
}

// toInputSchema maps a JSON-Schema object into the Anthropic tool schema param.
func toInputSchema(raw json.RawMessage) (anthropic.ToolInputSchemaParam, error) {
	var sc struct {
		Properties json.RawMessage `json:"properties"`
		Required   []string        `json:"required"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &sc); err != nil {
			return anthropic.ToolInputSchemaParam{}, err
		}
	}
	schema := anthropic.ToolInputSchemaParam{Required: sc.Required}
	if len(sc.Properties) > 0 {
		var props any
		if err := json.Unmarshal(sc.Properties, &props); err != nil {
			return anthropic.ToolInputSchemaParam{}, err
		}
		schema.Properties = props
	}
	return schema, nil
}

// renderToolResult converts an MCP tool result into the text that goes inside
// the tool_result block plus any extra content blocks (image/PDF) that must
// ride in the same user turn, since tool_result blocks carry text/image only.
// Evidence file content is wrapped in <untrusted_evidence> tags (design §5.2).
func renderToolResult(toolName string, res *mcp.CallToolResult) (string, []anthropic.ContentBlockParamUnion) {
	var sb strings.Builder
	var extra []anthropic.ContentBlockParamUnion

	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			sb.WriteString(v.Text)
			sb.WriteString("\n")
		case *mcp.ImageContent:
			extra = append(extra, anthropic.NewImageBlockBase64(
				v.MIMEType, base64.StdEncoding.EncodeToString(v.Data)))
			sb.WriteString("[image content attached below]\n")
		case *mcp.EmbeddedResource:
			if v.Resource != nil && v.Resource.MIMEType == "application/pdf" {
				extra = append(extra, anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{
					Data: base64.StdEncoding.EncodeToString(v.Resource.Blob),
				}))
				sb.WriteString("[PDF document attached below]\n")
			}
		}
	}

	text := strings.TrimRight(sb.String(), "\n")
	// File bytes are untrusted; frame them so the model treats them as data.
	if toolName == "get_evidence_file" && !res.IsError {
		text = "<untrusted_evidence>\n" + text + "\n</untrusted_evidence>"
	}
	if text == "" {
		text = "(no textual content)"
	}
	return text, extra
}

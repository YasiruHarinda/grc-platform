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
	"context"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/agent/mcpclient"
	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/ai-validation/internal/agent/task"
)

// loop drives the manual Anthropic tool loop (design §4.1.3). Each tool call is
// routed through the scoped MCP session, so the model never touches the entity
// directly and every call is scope-checked and logged server-side.
type loop struct {
	client       anthropic.Client
	defaultModel string
	maxIter      int
	log          *slog.Logger
}

// run executes the tool loop for one job. It returns submitted=true once the
// model has recorded a verdict via submit_validation_result (the terminal row
// is written by the MCP server at that point). A non-nil error is an
// infrastructure failure the caller may retry; submitted=false with err=nil
// means the model finished without producing a verdict.
func (l *loop) run(ctx context.Context, spec task.TaskSpec, scope task.Scope, sess *mcpclient.Session) (bool, error) {
	model := spec.Model
	if model == "" {
		model = l.defaultModel
	}
	maxIter := spec.MaxIterations
	if maxIter <= 0 {
		maxIter = l.maxIter
	}

	tools, err := bridgeTools(sess.Tools(), spec.AllowedTools)
	if err != nil {
		return false, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 16000,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		System: []anthropic.TextBlockParam{{
			Text:         spec.SystemPrompt(scope),
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Tools:    tools,
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(task.KickoffMessage))},
	}

	corrected := false
	for i := 0; i < maxIter; i++ {
		resp, err := l.client.Messages.New(ctx, params)
		if err != nil {
			return false, fmt.Errorf("anthropic message: %w", err)
		}
		l.log.Info("llm turn",
			"iter", i, "stopReason", resp.StopReason,
			"inputTokens", resp.Usage.InputTokens, "outputTokens", resp.Usage.OutputTokens,
			"cacheReadTokens", resp.Usage.CacheReadInputTokens)
		params.Messages = append(params.Messages, resp.ToParam())

		if resp.StopReason != anthropic.StopReasonToolUse {
			// Model ended its turn. If it never recorded a verdict, nudge once.
			if corrected {
				return false, nil
			}
			corrected = true
			params.Messages = append(params.Messages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(task.CorrectiveMessage)))
			continue
		}

		var results []anthropic.ContentBlockParamUnion
		submitted := false
		for _, block := range resp.Content {
			tu, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			res, callErr := sess.CallTool(ctx, tu.Name, tu.Input)
			if callErr != nil {
				// Protocol/infrastructure error → abort so the job may retry.
				return false, fmt.Errorf("mcp tool %q: %w", tu.Name, callErr)
			}
			text, extra := renderToolResult(tu.Name, res)
			results = append(results, anthropic.NewToolResultBlock(tu.ID, text, res.IsError))
			results = append(results, extra...)
			if tu.Name == "submit_validation_result" && !res.IsError {
				submitted = true
				break // session is now revoked; ignore any later blocks
			}
		}
		params.Messages = append(params.Messages, anthropic.NewUserMessage(results...))
		if submitted {
			return true, nil
		}
	}

	l.log.Warn("tool loop hit iteration cap without a verdict", "maxIter", maxIter)
	return false, nil
}

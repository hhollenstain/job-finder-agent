package llm

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

type anthropic struct {
	cfg    model.LLMConfig
	client *http.Client
}

func (a *anthropic) Complete(ctx context.Context, system, user string) (string, error) {
	resp, err := a.CompleteTools(ctx, system, []Message{{Role: "user", Content: user}}, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (a *anthropic) CompleteTools(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error) {
	msgs := make([]any, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			blocks := make([]any, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				blocks = append(blocks, map[string]string{"type": "text", "text": m.Content})
			}
			for _, call := range m.ToolCalls {
				var input any
				if err := json.Unmarshal([]byte(call.Arguments), &input); err != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    call.ID,
					"name":  call.Name,
					"input": input,
				})
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": blocks})
			continue
		}
		if m.Role == "tool" {
			msgs = append(msgs, map[string]any{
				"role": "user",
				"content": []any{map[string]string{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
			continue
		}
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, map[string]string{"role": role, "content": m.Content})
	}
	payload := map[string]any{
		"model":      a.cfg.Model,
		"max_tokens": 4096,
		"messages":   msgs,
	}
	if system != "" {
		payload["system"] = system
	}
	if len(tools) > 0 {
		spec := make([]any, 0, len(tools))
		for _, tool := range tools {
			spec = append(spec, map[string]any{
				"name":         tool.Name,
				"description":  tool.Description,
				"input_schema": tool.Schema,
			})
		}
		payload["tools"] = spec
	}
	body, err := postJSON(ctx, a.client, a.cfg.BaseURL+"/v1/messages", map[string]string{
		"x-api-key":         a.cfg.APIKey,
		"anthropic-version": "2023-06-01",
	}, payload)
	if err != nil {
		return Response{}, err
	}
	var parsed struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Response{}, err
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return Response{}, errString(parsed.Error.Message)
	}
	var out Response
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			if out.Content != "" {
				out.Content += "\n"
			}
			out.Content += block.Text
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: args})
		}
	}
	return out, nil
}

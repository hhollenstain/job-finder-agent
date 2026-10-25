package llm

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

type openAI struct {
	cfg    model.LLMConfig
	client *http.Client
}

func (o *openAI) Complete(ctx context.Context, system, user string) (string, error) {
	resp, err := o.CompleteTools(ctx, system, []Message{{Role: "user", Content: user}}, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (o *openAI) CompleteTools(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error) {
	msgs := make([]any, 0, len(messages)+1)
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	for _, m := range messages {
		switch m.Role {
		case "tool":
			msgs = append(msgs, map[string]string{
				"role":         "tool",
				"tool_call_id": m.ToolCallID,
				"content":      m.Content,
			})
		case "assistant":
			msg := map[string]any{"role": "assistant", "content": m.Content}
			if len(m.ToolCalls) > 0 {
				calls := make([]any, 0, len(m.ToolCalls))
				for _, call := range m.ToolCalls {
					calls = append(calls, map[string]any{
						"id":   call.ID,
						"type": "function",
						"function": map[string]string{
							"name":      call.Name,
							"arguments": call.Arguments,
						},
					})
				}
				msg["tool_calls"] = calls
			}
			msgs = append(msgs, msg)
		default:
			msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
		}
	}
	payload := map[string]any{
		"model":    o.cfg.Model,
		"messages": msgs,
	}
	if len(tools) > 0 {
		spec := make([]any, 0, len(tools))
		for _, tool := range tools {
			spec = append(spec, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.Schema,
				},
			})
		}
		payload["tools"] = spec
	}
	headers := map[string]string{}
	if o.cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + o.cfg.APIKey
	}
	body, err := postJSON(ctx, o.client, o.cfg.BaseURL+"/chat/completions", headers, payload)
	if err != nil {
		return Response{}, err
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
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
	if len(parsed.Choices) == 0 {
		return Response{}, errString("model returned no choices")
	}
	out := Response{Content: parsed.Choices[0].Message.Content}
	for _, call := range parsed.Choices[0].Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	return out, nil
}

type errString string

func (e errString) Error() string { return string(e) }

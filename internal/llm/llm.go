// Package llm talks to Anthropic and to any OpenAI-compatible chat API.
// Ollama, OpenAI, and a Cursor-compatible gateway all use the OpenAI client.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

// Tool is a function the model may call while expanding sources.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ToolCall is one model request to run a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Message is a chat turn. Tool results use Role "tool".
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// Response is a model completion, with tool calls when it asked for them.
type Response struct {
	Content   string
	ToolCalls []ToolCall
}

// Provider completes a prompt.
type Provider interface {
	Complete(ctx context.Context, system, user string) (string, error)
	CompleteTools(ctx context.Context, system string, messages []Message, tools []Tool) (Response, error)
}

// New builds a provider from settings. Ollama may omit an API key.
func New(cfg model.LLMConfig) (Provider, error) {
	cfg = normalize(cfg)
	switch cfg.Provider {
	case "anthropic":
		if strings.TrimSpace(cfg.APIKey) == "" {
			return nil, fmt.Errorf("anthropic api key is required")
		}
		return &anthropic{cfg: cfg, client: client()}, nil
	case "openai":
		if strings.TrimSpace(cfg.APIKey) == "" {
			return nil, fmt.Errorf("openai api key is required")
		}
		return &openAI{cfg: cfg, client: client()}, nil
	case "ollama":
		return &openAI{cfg: cfg, client: client()}, nil
	case "cursor", "compatible":
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return nil, fmt.Errorf("%s needs a base url for an OpenAI-compatible chat endpoint", cfg.Provider)
		}
		if cfg.Provider == "cursor" && strings.TrimSpace(cfg.APIKey) == "" {
			return nil, fmt.Errorf("cursor provider needs an api key for the gateway you configured")
		}
		return &openAI{cfg: cfg, client: client()}, nil
	default:
		return nil, fmt.Errorf("unknown model provider %q", cfg.Provider)
	}
}

func normalize(cfg model.LLMConfig) model.LLMConfig {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Model = strings.TrimSpace(cfg.Model)
	switch cfg.Provider {
	case "claude":
		cfg.Provider = "anthropic"
	case "openai":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
		if cfg.Model == "" {
			cfg.Model = "gpt-4o-mini"
		}
	case "anthropic":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.anthropic.com"
		}
		if cfg.Model == "" {
			cfg.Model = "claude-sonnet-4-5"
		}
	case "ollama":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://127.0.0.1:11434/v1"
		}
		if cfg.Model == "" {
			cfg.Model = "llama3.2"
		}
	}
	return cfg
}

func client() *http.Client {
	return &http.Client{Timeout: 90 * time.Second}
}

func do(ctx context.Context, client *http.Client, req *http.Request) ([]byte, error) {
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return nil, fmt.Errorf("model returned %s: %s", res.Status, msg)
	}
	return body, nil
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	return do(ctx, client, req)
}

// ExtractJSON returns the outermost JSON array or object in a model reply.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	startArr := strings.IndexByte(s, '[')
	startObj := strings.IndexByte(s, '{')
	if startArr < 0 && startObj < 0 {
		return s
	}
	start := startArr
	endCh := byte(']')
	if start < 0 || (startObj >= 0 && startObj < startArr) {
		start = startObj
		endCh = '}'
	}
	end := strings.LastIndexByte(s, endCh)
	if end < start {
		return s[start:]
	}
	return s[start : end+1]
}

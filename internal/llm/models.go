package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

// ListModels asks the configured provider for model ids the user can pick.
func ListModels(ctx context.Context, cfg model.LLMConfig) ([]string, error) {
	cfg = normalize(cfg)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	switch cfg.Provider {
	case "anthropic":
		return listAnthropicModels(ctx, cfg)
	case "ollama":
		models, err := listOllamaTags(ctx, cfg)
		if err == nil && len(models) > 0 {
			return models, nil
		}
		alt, err2 := listOpenAIModels(ctx, cfg)
		if err2 == nil && len(alt) > 0 {
			return alt, nil
		}
		if err != nil {
			return nil, err
		}
		return nil, err2
	default:
		return listOpenAIModels(ctx, cfg)
	}
}

func listOpenAIModels(ctx context.Context, cfg model.LLMConfig) ([]string, error) {
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/models"
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}
	body, err := getJSON(ctx, endpoint, headers)
	if err != nil {
		return nil, err
	}
	return parseModelList(body)
}

func listOllamaTags(ctx context.Context, cfg model.LLMConfig) ([]string, error) {
	root := strings.TrimRight(cfg.BaseURL, "/")
	root = strings.TrimSuffix(root, "/v1")
	body, err := getJSON(ctx, strings.TrimRight(root, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	return parseModelList(body)
}

func listAnthropicModels(ctx context.Context, cfg model.LLMConfig) ([]string, error) {
	base := strings.TrimRight(cfg.BaseURL, "/")
	endpoint := base + "/v1/models"
	if strings.HasSuffix(base, "/v1") {
		endpoint = base + "/models"
	}
	body, err := getJSON(ctx, endpoint, map[string]string{
		"x-api-key":         cfg.APIKey,
		"anthropic-version": "2023-06-01",
	})
	if err != nil {
		return nil, err
	}
	return parseModelList(body)
}

func getJSON(ctx context.Context, endpoint string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	return do(ctx, &http.Client{Timeout: 20 * time.Second}, req)
}

func parseModelList(body []byte) ([]string, error) {
	var doc struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, item := range doc.Data {
		add(item.ID)
	}
	for _, item := range doc.Models {
		add(item.Name)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, errString("provider returned no models")
	}
	return out, nil
}

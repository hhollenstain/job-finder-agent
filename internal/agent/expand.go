package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/llm"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/source"
)

// expand asks the model for up to three additional sources when the hard filter left a thin set.
func (a *Agent) expand(ctx context.Context, settings model.Settings, profile model.Profile, existing []model.Source, passCount int) []model.Source {
	if passCount >= 8 {
		return nil
	}
	provider, err := llm.New(settings.LLM)
	if err != nil {
		return nil
	}
	var catalog strings.Builder
	for _, kind := range source.Kinds() {
		fmt.Fprintf(&catalog, "- %s: %s", kind.Kind, kind.Summary)
		var fields []string
		for _, field := range kind.Fields {
			if field.Required {
				fields = append(fields, field.Name+" (required)")
			}
		}
		if len(fields) > 0 {
			fmt.Fprintf(&catalog, " Fields: %s.", strings.Join(fields, ", "))
		}
		catalog.WriteString("\n")
	}
	var have strings.Builder
	for _, src := range existing {
		fmt.Fprintf(&have, "- %s (%s)\n", src.Name, src.Kind)
	}
	system := `You add job sources for one person's search. Call add_source only for feeds or public boards that fit the profile. Do not invent API keys. Prefer remotive, remoteok, arbeitnow, greenhouse, lever, ashby, rss, or json. Skip a source that is already configured. Add at most 3.`
	user := fmt.Sprintf("Profile:\n%s\n\nKeywords: %s\n\nAlready configured:\n%s\nAvailable kinds:\n%s",
		profile.Notes, strings.Join(profile.Keywords, ", "), have.String(), catalog.String())
	tool := llm.Tool{
		Name:        "add_source",
		Description: "Register one job source the search can fetch on the next step.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind": map[string]any{"type": "string"},
				"name": map[string]any{"type": "string"},
				"config": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
				},
			},
			"required": []string{"kind", "name"},
		},
	}
	resp, err := provider.CompleteTools(ctx, system, []llm.Message{{Role: "user", Content: user}}, []llm.Tool{tool})
	if err != nil {
		return nil
	}
	var added []model.Source
	now := time.Now().UTC()
	for _, call := range resp.ToolCalls {
		if call.Name != "add_source" || len(added) >= 3 {
			continue
		}
		src, err := sourceFromTool(call.Arguments, now)
		if err != nil {
			continue
		}
		dup := false
		for _, old := range existing {
			if source.SameIdentity(old, src) || (old.Kind == src.Kind && old.Name == src.Name) {
				dup = true
				break
			}
		}
		for _, old := range added {
			if source.SameIdentity(old, src) {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		if err := a.store.InsertSource(src); err != nil {
			continue
		}
		added = append(added, src)
	}
	return added
}

func sourceFromTool(raw string, now time.Time) (model.Source, error) {
	var body struct {
		Kind   string          `json:"kind"`
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return model.Source{}, err
	}
	cfg := map[string]string{}
	if len(body.Config) > 0 && string(body.Config) != "null" {
		var generic map[string]any
		if err := json.Unmarshal(body.Config, &generic); err != nil {
			return model.Source{}, err
		}
		for k, v := range generic {
			if v == nil {
				continue
			}
			switch t := v.(type) {
			case string:
				cfg[k] = t
			default:
				cfg[k] = fmt.Sprint(t)
			}
		}
	}
	body.Kind = strings.TrimSpace(body.Kind)
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 80 {
		return model.Source{}, fmt.Errorf("name is required")
	}
	if err := source.Validate(body.Kind, cfg); err != nil {
		return model.Source{}, err
	}
	return model.Source{
		ID:        newID("src_"),
		Kind:      body.Kind,
		Name:      body.Name,
		Enabled:   true,
		Config:    cfg,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

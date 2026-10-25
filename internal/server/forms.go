package server

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

type sourceBody struct {
	ID      string            `json:"id"`
	Kind    string            `json:"kind"`
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config"`
}

func (b sourceBody) source(now time.Time) (model.Source, error) {
	kind := strings.TrimSpace(b.Kind)
	name := strings.TrimSpace(b.Name)
	if kind == "" {
		return model.Source{}, fmt.Errorf("kind is required")
	}
	if name == "" {
		name = kind
	}
	if len(name) > 80 {
		return model.Source{}, fmt.Errorf("name is too long")
	}
	id := strings.TrimSpace(b.ID)
	if id == "" {
		id = "src_" + textutil.Hash(kind, name, now.Format(time.RFC3339Nano))[:8]
	}
	if !idRE.MatchString(id) {
		return model.Source{}, fmt.Errorf("id must be letters, numbers, _ or -")
	}
	cfg := b.Config
	if cfg == nil {
		cfg = map[string]string{}
	}
	return model.Source{
		ID: id, Kind: kind, Name: name, Enabled: b.Enabled, Config: cfg,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func cleanProfile(p model.Profile) model.Profile {
	p.Notes = textutil.Truncate(strings.TrimSpace(p.Notes), 8000)
	p.Keywords = textutil.CleanList(p.Keywords, 40)
	p.MustHave = textutil.CleanList(p.MustHave, 40)
	p.Exclude = textutil.CleanList(p.Exclude, 40)
	p.Locations = textutil.CleanList(p.Locations, 20)
	p.EmploymentTypes = textutil.CleanList(p.EmploymentTypes, 8)
	if p.MinScore < 0 {
		p.MinScore = 0
	}
	if p.MinScore > 100 {
		p.MinScore = 100
	}
	if p.MinSalary < 0 {
		p.MinSalary = 0
	}
	return p
}

func cleanSettings(s model.Settings) model.Settings {
	s.LLM.Provider = strings.ToLower(strings.TrimSpace(s.LLM.Provider))
	if s.LLM.Provider == "" {
		s.LLM.Provider = "ollama"
	}
	s.LLM.BaseURL = strings.TrimRight(strings.TrimSpace(s.LLM.BaseURL), "/")
	s.LLM.Model = strings.TrimSpace(s.LLM.Model)
	s.Kiwi.BaseURL = strings.TrimRight(strings.TrimSpace(s.Kiwi.BaseURL), "/")
	s.Kiwi.Space = strings.TrimSpace(s.Kiwi.Space)
	if prefix, err := textutil.SafePathPrefix(s.Kiwi.PathPrefix); err == nil {
		s.Kiwi.PathPrefix = prefix
	} else {
		s.Kiwi.PathPrefix = "job-search"
	}
	if s.MaxLLMJobs <= 0 {
		s.MaxLLMJobs = 25
	}
	if s.MaxLLMJobs > 100 {
		s.MaxLLMJobs = 100
	}
	if s.CacheHours < 0 {
		s.CacheHours = 0
	}
	if s.CacheHours > 168 {
		s.CacheHours = 168
	}
	return s
}

type settingsView struct {
	LLM           llmView  `json:"llm"`
	Kiwi          kiwiView `json:"kiwi"`
	Schedule      string   `json:"schedule"`
	Timezone      string   `json:"timezone"`
	ScoreWithLLM  bool     `json:"score_with_llm"`
	AutoResearch  bool     `json:"auto_research"`
	ExpandSources bool     `json:"expand_sources"`
	MaxLLMJobs    int      `json:"max_llm_jobs"`
	CacheHours    int      `json:"cache_hours"`
}

type llmView struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	KeySet   bool   `json:"key_set"`
}

type kiwiView struct {
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Space      string `json:"space"`
	PathPrefix string `json:"path_prefix"`
	KeySet     bool   `json:"key_set"`
}

func viewSettings(s model.Settings) settingsView {
	return settingsView{
		LLM: llmView{
			Provider: s.LLM.Provider,
			BaseURL:  s.LLM.BaseURL,
			APIKey:   mask(s.LLM.APIKey),
			Model:    s.LLM.Model,
			KeySet:   s.LLM.APIKey != "",
		},
		Kiwi: kiwiView{
			Enabled:    s.Kiwi.Enabled,
			BaseURL:    s.Kiwi.BaseURL,
			APIKey:     mask(s.Kiwi.APIKey),
			Space:      s.Kiwi.Space,
			PathPrefix: s.Kiwi.PathPrefix,
			KeySet:     s.Kiwi.APIKey != "",
		},
		Schedule:      s.Schedule,
		Timezone:      s.Timezone,
		ScoreWithLLM:  s.ScoreWithLLM,
		AutoResearch:  s.AutoResearch,
		ExpandSources: s.ExpandSources,
		MaxLLMJobs:    s.MaxLLMJobs,
		CacheHours:    s.CacheHours,
	}
}

func mergeSecret(current, incoming string) string {
	incoming = strings.TrimSpace(incoming)
	if incoming == "" || strings.HasPrefix(incoming, "••••") {
		return current
	}
	return incoming
}

func isSecret(key string) bool {
	switch strings.ToLower(key) {
	case "api_key", "app_key", "token", "authorization":
		return true
	default:
		return false
	}
}

func mask(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "••••"
	}
	return "••••" + v[len(v)-4:]
}

func maskConfig(cfg map[string]string) map[string]string {
	out := make(map[string]string, len(cfg))
	for k, v := range cfg {
		if isSecret(k) {
			out[k] = mask(v)
			continue
		}
		out[k] = v
	}
	return out
}

func mergeConfig(current, incoming map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range current {
		out[k] = v
	}
	for k, v := range incoming {
		if isSecret(k) {
			out[k] = mergeSecret(current[k], v)
			continue
		}
		out[k] = v
	}
	return out
}

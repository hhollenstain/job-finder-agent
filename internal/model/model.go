// Package model holds the job-finder domain types shared by the store, sources, and UI.
package model

import "time"

const (
	StatusNew       = "new"
	StatusMatched   = "matched"
	StatusRejected  = "rejected"
	StatusSaved     = "saved"
	StatusDismissed = "dismissed"

	TriggerManual   = "manual"
	TriggerStartup  = "startup"
	TriggerSchedule = "schedule"

	RunRunning = "running"
	RunOK      = "ok"
	RunError   = "error"
)

// Profile is the search contract. Keyword checks are deterministic.
// Notes and must-haves are also handed to the model when scoring is on.
type Profile struct {
	Notes           string   `json:"notes" yaml:"notes"`
	Keywords        []string `json:"keywords" yaml:"keywords"`
	MustHave        []string `json:"must_have" yaml:"must_have"`
	Exclude         []string `json:"exclude" yaml:"exclude"`
	Locations       []string `json:"locations" yaml:"locations"`
	RemoteOnly      bool     `json:"remote_only" yaml:"remote_only"`
	IncludeRemote   bool     `json:"include_remote" yaml:"include_remote"`
	MinSalary       int      `json:"min_salary" yaml:"min_salary"`
	MinScore        int      `json:"min_score" yaml:"min_score"`
	EmploymentTypes []string `json:"employment_types" yaml:"employment_types"`
}

// DefaultProfile is used the first time the database is created.
func DefaultProfile() Profile {
	return Profile{
		Notes: "Backend engineer. Prefer Go, clear writing, and a product the company can explain.",
		Keywords: []string{
			"go",
			"golang",
			"backend",
		},
		Exclude:       []string{"unpaid", "internship"},
		IncludeRemote: true,
		MinScore:      70,
	}
}

// LLMConfig selects which provider scores and researches roles.
type LLMConfig struct {
	Provider string `json:"provider" yaml:"provider"`
	BaseURL  string `json:"base_url" yaml:"base_url"`
	APIKey   string `json:"api_key,omitempty" yaml:"api_key"`
	Model    string `json:"model" yaml:"model"`
}

// KiwiConfig is optional. When enabled, matched jobs are also written to a KiwiFS workspace.
type KiwiConfig struct {
	Enabled    bool   `json:"enabled" yaml:"enabled"`
	BaseURL    string `json:"base_url" yaml:"base_url"`
	APIKey     string `json:"api_key,omitempty" yaml:"api_key"`
	Space      string `json:"space" yaml:"space"`
	PathPrefix string `json:"path_prefix" yaml:"path_prefix"`
}

// Settings is persisted as one JSON document.
type Settings struct {
	LLM           LLMConfig  `json:"llm" yaml:"llm"`
	Kiwi          KiwiConfig `json:"kiwi" yaml:"kiwi"`
	Schedule      string     `json:"schedule" yaml:"schedule"`
	Timezone      string     `json:"timezone" yaml:"timezone"`
	ScoreWithLLM  bool       `json:"score_with_llm" yaml:"score_with_llm"`
	AutoResearch  bool       `json:"auto_research" yaml:"auto_research"`
	ExpandSources bool       `json:"expand_sources" yaml:"expand_sources"`
	MaxLLMJobs    int        `json:"max_llm_jobs" yaml:"max_llm_jobs"`
	CacheHours    int        `json:"cache_hours" yaml:"cache_hours"`
}

// DefaultSettings prefers a local Ollama endpoint and does not call it until scoring is turned on.
func DefaultSettings() Settings {
	return Settings{
		LLM: LLMConfig{
			Provider: "ollama",
			BaseURL:  "http://127.0.0.1:11434/v1",
			Model:    "llama3.2",
		},
		Kiwi: KiwiConfig{
			BaseURL:    "http://127.0.0.1:3333",
			PathPrefix: "job-search",
		},
		Timezone:   "Local",
		MaxLLMJobs: 25,
		CacheHours: 6,
	}
}

// Source is one configured search endpoint. Kind selects the adapter.
type Source struct {
	ID        string            `json:"id" yaml:"id"`
	Kind      string            `json:"kind" yaml:"kind"`
	Name      string            `json:"name" yaml:"name"`
	Enabled   bool              `json:"enabled" yaml:"enabled"`
	Config    map[string]string `json:"config" yaml:"config"`
	CreatedAt time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time         `json:"updated_at" yaml:"updated_at"`
}

// JobDraft is a posting before it is filtered or stored.
type JobDraft struct {
	ExternalID     string     `json:"external_id"`
	Title          string     `json:"title"`
	Company        string     `json:"company"`
	Location       string     `json:"location"`
	Remote         *bool      `json:"remote,omitempty"`
	URL            string     `json:"url"`
	Description    string     `json:"description"`
	SalaryMin      int        `json:"salary_min"`
	SalaryMax      int        `json:"salary_max"`
	Currency       string     `json:"currency"`
	EmploymentType string     `json:"employment_type"`
	PostedAt       *time.Time `json:"posted_at,omitempty"`
}

// Job is a stored posting plus the filter and model outcome.
type Job struct {
	ID             string     `json:"id"`
	SourceID       string     `json:"source_id"`
	SourceName     string     `json:"source_name,omitempty"`
	SourceKind     string     `json:"source_kind"`
	ExternalID     string     `json:"external_id"`
	Fingerprint    string     `json:"fingerprint"`
	Title          string     `json:"title"`
	Company        string     `json:"company"`
	Location       string     `json:"location"`
	Remote         bool       `json:"remote"`
	URL            string     `json:"url"`
	Description    string     `json:"description,omitempty"`
	SalaryMin      int        `json:"salary_min"`
	SalaryMax      int        `json:"salary_max"`
	Currency       string     `json:"currency"`
	EmploymentType string     `json:"employment_type"`
	PostedAt       *time.Time `json:"posted_at,omitempty"`
	Status         string     `json:"status"`
	Favorite       bool       `json:"favorite"`
	Score          int        `json:"score"`
	FilterReason   string     `json:"filter_reason"`
	FitSummary     string     `json:"fit_summary"`
	FitPros        []string   `json:"fit_pros,omitempty"`
	FitCons        []string   `json:"fit_cons,omitempty"`
	Research       string     `json:"research,omitempty"`
	FirstSeen      time.Time  `json:"first_seen"`
	LastSeen       time.Time  `json:"last_seen"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ListItem is the jobs table payload. The full description stays on the detail route.
type ListItem struct {
	ID             string     `json:"id"`
	SourceID       string     `json:"source_id"`
	SourceName     string     `json:"source_name,omitempty"`
	SourceKind     string     `json:"source_kind"`
	Title          string     `json:"title"`
	Company        string     `json:"company"`
	Location       string     `json:"location"`
	Remote         bool       `json:"remote"`
	URL            string     `json:"url"`
	SalaryMin      int        `json:"salary_min"`
	SalaryMax      int        `json:"salary_max"`
	Currency       string     `json:"currency"`
	EmploymentType string     `json:"employment_type"`
	PostedAt       *time.Time `json:"posted_at,omitempty"`
	Status         string     `json:"status"`
	Favorite       bool       `json:"favorite"`
	Score          int        `json:"score"`
	FilterReason   string     `json:"filter_reason"`
	FitSummary     string     `json:"fit_summary"`
	FitPros        []string   `json:"fit_pros,omitempty"`
	FitCons        []string   `json:"fit_cons,omitempty"`
	HasResearch    bool       `json:"has_research"`
	FirstSeen      time.Time  `json:"first_seen"`
	LastSeen       time.Time  `json:"last_seen"`
}

// Summary copies the fields the list view needs.
func (j Job) Summary() ListItem {
	return ListItem{
		ID:             j.ID,
		SourceID:       j.SourceID,
		SourceName:     j.SourceName,
		SourceKind:     j.SourceKind,
		Title:          j.Title,
		Company:        j.Company,
		Location:       j.Location,
		Remote:         j.Remote,
		URL:            j.URL,
		SalaryMin:      j.SalaryMin,
		SalaryMax:      j.SalaryMax,
		Currency:       j.Currency,
		EmploymentType: j.EmploymentType,
		PostedAt:       j.PostedAt,
		Status:         j.Status,
		Favorite:       j.Favorite,
		Score:          j.Score,
		FilterReason:   j.FilterReason,
		FitSummary:     j.FitSummary,
		FitPros:        j.FitPros,
		FitCons:        j.FitCons,
		HasResearch:    j.Research != "",
		FirstSeen:      j.FirstSeen,
		LastSeen:       j.LastSeen,
	}
}

// Run is one search execution, manual or scheduled.
type Run struct {
	ID         string     `json:"id"`
	Trigger    string     `json:"trigger"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Fetched    int        `json:"fetched"`
	New        int        `json:"new"`
	Matched    int        `json:"matched"`
	Rejected   int        `json:"rejected"`
	Message    string     `json:"message"`
}

// SyncStatus records the last context-filesystem write.
type SyncStatus struct {
	At       time.Time `json:"at"`
	LocalDir string    `json:"local_dir"`
	Files    int       `json:"files"`
	Kiwi     string    `json:"kiwi"`
}

// Field describes one source configuration input for the web UI.
type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Help        string `json:"help,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// KindInfo is a source adapter the user or the agent can instantiate.
type KindInfo struct {
	Kind    string  `json:"kind"`
	Label   string  `json:"label"`
	Summary string  `json:"summary"`
	Public  bool    `json:"public"`
	Fields  []Field `json:"fields"`
}

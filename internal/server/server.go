// Package server is the HTTP API and the embedded web UI.
package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/agent"
	"github.com/henryhollenstain/job-finder-agent/internal/kiwi"
	"github.com/henryhollenstain/job-finder-agent/internal/llm"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/schedule"
	"github.com/henryhollenstain/job-finder-agent/internal/source"
	"github.com/henryhollenstain/job-finder-agent/internal/store"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
	"github.com/henryhollenstain/job-finder-agent/web"
)

// Server serves the API and the browser UI.
type Server struct {
	store   *store.Store
	agent   *agent.Agent
	sched   *schedule.Runner
	dataDir string
}

// New wires the HTTP handlers.
func New(st *store.Store, ag *agent.Agent, sched *schedule.Runner, dataDir string) *Server {
	return &Server{store: st, agent: ag, sched: sched, dataDir: dataDir}
}

// Handler is the application router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/profile", s.getProfile)
	mux.HandleFunc("PUT /api/profile", s.putProfile)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/schedule", s.getSchedule)
	mux.HandleFunc("PUT /api/schedule", s.putSchedule)
	mux.HandleFunc("GET /api/source-kinds", s.sourceKinds)
	mux.HandleFunc("GET /api/sources", s.listSources)
	mux.HandleFunc("POST /api/sources", s.createSource)
	mux.HandleFunc("PUT /api/sources/{id}", s.updateSource)
	mux.HandleFunc("DELETE /api/sources/{id}", s.deleteSource)
	mux.HandleFunc("POST /api/runs", s.startRun)
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("POST /api/models", s.listModels)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	mux.HandleFunc("PATCH /api/jobs/{id}", s.patchJob)
	mux.HandleFunc("POST /api/jobs/purge", s.purgeJobs)
	mux.HandleFunc("POST /api/jobs/{id}/research", s.researchJob)
	mux.HandleFunc("GET /api/context", s.getContext)
	mux.HandleFunc("POST /api/context/sync", s.syncContext)
	mux.Handle("GET /", noCache(http.FileServer(http.FS(web.Files))))
	return mux
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	counts, err := s.store.CountByStatus()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	settings, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	body := map[string]any{
		"counts":       counts,
		"schedule":     settings.Schedule,
		"timezone":     settings.Timezone,
		"next":         s.sched.Next(),
		"llm_ready":    llmReady(settings.LLM),
		"provider":     settings.LLM.Provider,
		"model":        settings.LLM.Model,
		"score":        settings.ScoreWithLLM,
		"expand":       settings.ExpandSources,
		"kiwi_enabled": settings.Kiwi.Enabled,
	}
	if run, ok, err := s.store.ActiveRun(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	} else if ok {
		body["active"] = run
	}
	if run, ok, err := s.store.LatestRun(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	} else if ok {
		body["last"] = run
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.Profile()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) putProfile(w http.ResponseWriter, r *http.Request) {
	var p model.Profile
	if !decode(w, r, &p) {
		return
	}
	p = cleanProfile(p)
	if err := s.store.SaveProfile(p); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, viewSettings(cfg))
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in model.Settings
	if !decode(w, r, &in) {
		return
	}
	cur, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	in.LLM.APIKey = mergeSecret(cur.LLM.APIKey, in.LLM.APIKey)
	in.Kiwi.APIKey = mergeSecret(cur.Kiwi.APIKey, in.Kiwi.APIKey)
	in.Schedule = cur.Schedule
	in.Timezone = cur.Timezone
	in = cleanSettings(in)
	if err := s.store.SaveSettings(in); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, viewSettings(in))
}

func (s *Server) getSchedule(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schedule": cfg.Schedule,
		"timezone": cfg.Timezone,
		"next":     s.sched.Next(),
	})
}

func (s *Server) putSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Schedule string `json:"schedule"`
		Timezone string `json:"timezone"`
	}
	if !decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Timezone) == "" {
		body.Timezone = "Local"
	}
	if err := s.sched.Apply(body.Schedule, body.Timezone); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	cfg.Schedule = strings.TrimSpace(body.Schedule)
	cfg.Timezone = body.Timezone
	if err := s.store.SaveSettings(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schedule": cfg.Schedule,
		"timezone": cfg.Timezone,
		"next":     s.sched.Next(),
	})
}

func (s *Server) sourceKinds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"kinds": source.Kinds()})
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListSources()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	for i := range items {
		items[i].Config = maskConfig(items[i].Config)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": items})
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	var in sourceBody
	if !decode(w, r, &in) {
		return
	}
	src, err := in.source(time.Now().UTC())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := source.Validate(src.Kind, src.Config); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.InsertSource(src); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	src.Config = maskConfig(src.Config)
	writeJSON(w, http.StatusCreated, src)
}

func (s *Server) updateSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cur, err := s.store.GetSource(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("source not found"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var in sourceBody
	if !decode(w, r, &in) {
		return
	}
	next := cur
	if strings.TrimSpace(in.Name) != "" {
		next.Name = strings.TrimSpace(in.Name)
	}
	if in.Kind != "" {
		next.Kind = in.Kind
	}
	next.Enabled = in.Enabled
	if in.Config != nil {
		next.Config = mergeConfig(cur.Config, in.Config)
	}
	next.UpdatedAt = time.Now().UTC()
	if err := source.Validate(next.Kind, next.Config); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateSource(next); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	next.Config = maskConfig(next.Config)
	writeJSON(w, http.StatusOK, next)
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteSource(id); errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("source not found"))
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Force bool `json:"force"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if !decode(w, r, &body) {
			return
		}
	}
	run, err := s.agent.Start(model.TriggerManual, body.Force)
	if errors.Is(err, agent.ErrBusy) {
		writeErr(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if runs == nil {
		runs = []model.Run{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRun(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("run not found"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
	}
	if !decode(w, r, &body) {
		return
	}
	cur, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	cfg := model.LLMConfig{
		Provider: strings.ToLower(strings.TrimSpace(body.Provider)),
		BaseURL:  strings.TrimSpace(body.BaseURL),
		APIKey:   strings.TrimSpace(body.APIKey),
		Model:    cur.LLM.Model,
	}
	if cfg.Provider == "" {
		cfg.Provider = cur.LLM.Provider
	}
	if cfg.Provider == "claude" {
		cfg.Provider = "anthropic"
	}
	if cfg.Provider == strings.ToLower(strings.TrimSpace(cur.LLM.Provider)) {
		cfg.APIKey = mergeSecret(cur.LLM.APIKey, body.APIKey)
		if cfg.BaseURL == "" {
			cfg.BaseURL = cur.LLM.BaseURL
		}
	} else if strings.HasPrefix(cfg.APIKey, "••••") {
		cfg.APIKey = ""
	}
	switch cfg.Provider {
	case "ollama":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://127.0.0.1:11434/v1"
		}
	case "openai":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
	case "anthropic":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.anthropic.com"
		}
	}
	if err := source.CheckURL(cfg.BaseURL); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	models, err := llm.ListModels(r.Context(), cfg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func sourceIDs(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] || len(out) >= 40 || !validSourceID(part) {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func validSourceID(id string) bool {
	if len(id) > 80 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var statuses []string
	if raw := strings.TrimSpace(q.Get("status")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				statuses = append(statuses, part)
			}
		}
	}
	limit := 200
	fav := strings.TrimSpace(q.Get("favorite"))
	jobs, err := s.store.ListJobs(store.JobFilter{
		Statuses:  statuses,
		SourceIDs: sourceIDs(q.Get("source")),
		Query:     q.Get("q"),
		Sort:      q.Get("sort"),
		Favorite:  fav == "1" || strings.EqualFold(fav, "true"),
		Limit:     limit,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]model.ListItem, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, job.Summary())
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.GetJob(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("job not found"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) patchJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status   *string `json:"status"`
		Favorite *bool   `json:"favorite"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Status == nil && body.Favorite == nil {
		writeErr(w, http.StatusBadRequest, errors.New("status or favorite is required"))
		return
	}
	id := r.PathValue("id")
	if body.Status != nil {
		if err := s.store.UpdateJobStatus(id, *body.Status); errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, errors.New("job not found"))
			return
		} else if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	if body.Favorite != nil {
		if err := s.store.SetFavorite(id, *body.Favorite); errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, errors.New("job not found"))
			return
		} else if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	job, err := s.store.GetJob(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) purgeJobs(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.PurgeUnkept()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if ids, err := s.store.JobIDs(); err == nil {
		keep := make(map[string]bool, len(ids))
		for _, id := range ids {
			keep[id] = true
		}
		prefix := "job-search"
		if cfg, err := s.store.Settings(); err == nil {
			if safe, err := textutil.SafePathPrefix(cfg.Kiwi.PathPrefix); err == nil {
				prefix = safe
			}
		}
		_, _ = kiwi.PruneJobPages(filepath.Join(s.dataDir, "context"), prefix, keep)
	}
	_, _ = s.agent.Sync(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"removed": n})
}

func (s *Server) researchJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.agent.ResearchJob(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, errors.New("job not found"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) getContext(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.Settings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	last, err := s.store.LastSync()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	prefix, err := textutil.SafePathPrefix(cfg.Kiwi.PathPrefix)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		rel = "index.md"
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		writeErr(w, http.StatusBadRequest, errors.New("context pages are markdown"))
		return
	}
	safe, err := textutil.SafeRel(prefix, rel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	root := filepath.Clean(filepath.Join(s.dataDir, "context"))
	full := filepath.Clean(filepath.Join(root, filepath.FromSlash(safe)))
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		writeErr(w, http.StatusBadRequest, errors.New("context path escapes the folder"))
		return
	}
	preview := ""
	if b, err := os.ReadFile(full); err == nil {
		preview = string(b)
		const limit = 120000
		if len(preview) > limit {
			cut := strings.LastIndex(preview[:limit], "\n")
			if cut < 0 {
				cut = limit
			}
			preview = preview[:cut] + "\n\n…\n"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kiwi":      viewSettings(cfg).Kiwi,
		"last_sync": last,
		"path":      rel,
		"preview":   preview,
		"local_dir": filepath.Join(s.dataDir, "context", filepath.FromSlash(prefix)),
	})
}

func (s *Server) syncContext(w http.ResponseWriter, r *http.Request) {
	status, err := s.agent.Sync(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func llmReady(cfg model.LLMConfig) bool {
	_, err := llm.New(cfg)
	return err == nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dest); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

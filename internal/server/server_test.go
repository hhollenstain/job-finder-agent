package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/agent"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/schedule"
	"github.com/henryhollenstain/job-finder-agent/internal/store"
)

func newTest(t *testing.T) (*httptest.Server, *store.Store, *schedule.Runner) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ag := agent.New(st, filepath.Join(dir, "data"))
	runner := schedule.New(ag)
	t.Cleanup(func() { _ = runner.Apply("", "UTC") })
	srv := httptest.NewServer(New(st, ag, runner, filepath.Join(dir, "data")).Handler())
	t.Cleanup(srv.Close)
	return srv, st, runner
}

func TestProfileRoundTrip(t *testing.T) {
	srv, _, _ := newTest(t)
	body := model.Profile{Notes: "Go roles", Keywords: []string{"golang"}, IncludeRemote: true, MinScore: 70}
	res := do(t, srv, http.MethodPut, "/api/profile", body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d %s", res.StatusCode, read(t, res))
	}
	res = do(t, srv, http.MethodGet, "/api/profile", nil)
	var got model.Profile
	mustDecode(t, res, &got)
	if len(got.Keywords) != 1 || got.Keywords[0] != "golang" || got.Notes != "Go roles" {
		t.Fatalf("%#v", got)
	}
}

func TestScheduleAndSecretMask(t *testing.T) {
	srv, _, _ := newTest(t)
	res := do(t, srv, http.MethodPut, "/api/schedule", map[string]string{"schedule": "nope", "timezone": "UTC"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	res = do(t, srv, http.MethodPut, "/api/schedule", map[string]string{"schedule": "0 8 * * 1-5", "timezone": "UTC"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("schedule %d %s", res.StatusCode, read(t, res))
	}

	res = do(t, srv, http.MethodPut, "/api/settings", model.Settings{
		LLM:        model.LLMConfig{Provider: "openai", APIKey: "sk-test-1234", Model: "gpt-4o-mini"},
		Kiwi:       model.KiwiConfig{PathPrefix: "job-search"},
		MaxLLMJobs: 10,
		CacheHours: 2,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("settings %d %s", res.StatusCode, read(t, res))
	}
	var view map[string]any
	mustDecode(t, res, &view)
	llm := view["llm"].(map[string]any)
	if llm["api_key"] != "••••1234" {
		t.Fatalf("key leaked: %#v", llm)
	}
	res = do(t, srv, http.MethodPut, "/api/settings", model.Settings{
		LLM:        model.LLMConfig{Provider: "openai", APIKey: "••••1234", Model: "gpt-4o-mini"},
		Kiwi:       model.KiwiConfig{PathPrefix: "job-search"},
		MaxLLMJobs: 10,
		CacheHours: 2,
	})
	mustDecode(t, res, &view)
	llm = view["llm"].(map[string]any)
	if llm["api_key"] != "••••1234" {
		t.Fatalf("mask overwrite: %#v", llm)
	}
}

func TestSourceValidationAndJobs(t *testing.T) {
	srv, st, _ := newTest(t)
	res := do(t, srv, http.MethodPost, "/api/sources", map[string]any{
		"kind": "json", "name": "bad", "config": map[string]string{"url": "http://169.254.169.254/latest"},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected reject metadata, got %d %s", res.StatusCode, read(t, res))
	}
	now := time.Now().UTC()
	job := model.Job{
		ID: "job_test", SourceID: "remotive", SourceKind: "remotive", ExternalID: "1",
		Fingerprint: "fp1", Title: "Go engineer", Company: "North", Location: "Remote",
		Remote: true, URL: "https://example.com/jobs/1", Description: "Distributed systems",
		Status: model.StatusMatched, Score: 82, FitSummary: "Fits the Go work",
		FitPros: []string{"Mentions Go"}, FitCons: []string{"Pay is not listed"},
		FirstSeen: now, LastSeen: now, UpdatedAt: now,
	}
	if _, err := st.UpsertJob(job); err != nil {
		t.Fatal(err)
	}
	res = do(t, srv, http.MethodGet, "/api/jobs?status=matched", nil)
	var list struct {
		Jobs []model.ListItem `json:"jobs"`
	}
	mustDecode(t, res, &list)
	if len(list.Jobs) != 1 || list.Jobs[0].Title != "Go engineer" || list.Jobs[0].HasResearch {
		t.Fatalf("%#v", list.Jobs)
	}
	if len(list.Jobs[0].FitPros) != 1 || list.Jobs[0].FitPros[0] != "Mentions Go" || len(list.Jobs[0].FitCons) != 1 {
		t.Fatalf("fit points %#v", list.Jobs[0])
	}
	res = do(t, srv, http.MethodPatch, "/api/jobs/job_test", map[string]any{"status": "saved"})
	var saved model.Job
	mustDecode(t, res, &saved)
	if saved.Status != model.StatusSaved {
		t.Fatalf("save %#v", saved.Status)
	}
	res = do(t, srv, http.MethodPatch, "/api/jobs/job_test", map[string]any{"status": "matched"})
	mustDecode(t, res, &saved)
	if saved.Status != model.StatusMatched {
		t.Fatalf("unsave %#v", saved.Status)
	}
	res = do(t, srv, http.MethodPatch, "/api/jobs/job_test", map[string]any{"favorite": true})
	mustDecode(t, res, &saved)
	if !saved.Favorite {
		t.Fatal("expected favorite")
	}
	res = do(t, srv, http.MethodGet, "/api/jobs?favorite=1", nil)
	mustDecode(t, res, &list)
	if len(list.Jobs) != 1 || !list.Jobs[0].Favorite {
		t.Fatalf("favorite list %#v", list.Jobs)
	}
	res = do(t, srv, http.MethodGet, "/", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ui %d", res.StatusCode)
	}
	html, _ := io.ReadAll(res.Body)
	if !bytes.Contains(html, []byte("Job Finder")) {
		t.Fatal("missing ui")
	}
}

func TestPurgeKeepsSavedAndFavorites(t *testing.T) {
	srv, st, _ := newTest(t)
	now := time.Now().UTC()
	insert := func(id, status string, favorite bool) {
		t.Helper()
		job := model.Job{
			ID: id, SourceID: "src", SourceKind: "json", ExternalID: id, Fingerprint: id,
			Title: id, Company: "Co", URL: "https://example.com/" + id,
			Status: status, Favorite: favorite, Score: 50,
			FirstSeen: now, LastSeen: now, UpdatedAt: now,
		}
		if _, err := st.UpsertJob(job); err != nil {
			t.Fatal(err)
		}
		if favorite {
			if err := st.SetFavorite(id, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert("job_saved", model.StatusSaved, false)
	insert("job_star", model.StatusRejected, true)
	insert("job_open", model.StatusMatched, false)
	insert("job_drop", model.StatusDismissed, false)

	res := do(t, srv, http.MethodGet, "/api/overview", nil)
	var overview struct {
		Counts map[string]int `json:"counts"`
	}
	mustDecode(t, res, &overview)
	if overview.Counts["purgeable"] != 2 {
		t.Fatalf("purgeable %d", overview.Counts["purgeable"])
	}

	res = do(t, srv, http.MethodPost, "/api/jobs/purge", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("purge %d %s", res.StatusCode, read(t, res))
	}
	var body struct {
		Removed int `json:"removed"`
	}
	mustDecode(t, res, &body)
	if body.Removed != 2 {
		t.Fatalf("removed %d", body.Removed)
	}

	res = do(t, srv, http.MethodGet, "/api/jobs", nil)
	var list struct {
		Jobs []model.ListItem `json:"jobs"`
	}
	mustDecode(t, res, &list)
	got := map[string]bool{}
	for _, job := range list.Jobs {
		got[job.ID] = true
	}
	if !got["job_saved"] || !got["job_star"] || got["job_open"] || got["job_drop"] || len(list.Jobs) != 2 {
		t.Fatalf("left %#v", got)
	}
}

func TestModelListAndPaySort(t *testing.T) {
	var auth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"zeta"},{"id":"alpha"}]}`))
	}))
	defer upstream.Close()

	srv, st, _ := newTest(t)
	res := do(t, srv, http.MethodPut, "/api/settings", model.Settings{
		LLM: model.LLMConfig{
			Provider: "openai",
			BaseURL:  upstream.URL + "/v1",
			Model:    "kept-model",
			APIKey:   "sk-test-1234",
		},
		Kiwi:       model.KiwiConfig{PathPrefix: "job-search"},
		MaxLLMJobs: 10,
		CacheHours: 2,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("save %d %s", res.StatusCode, read(t, res))
	}
	res.Body.Close()
	res = do(t, srv, http.MethodGet, "/api/settings", nil)
	var view map[string]any
	mustDecode(t, res, &view)
	llm := view["llm"].(map[string]any)
	if llm["provider"] != "openai" || llm["base_url"] != upstream.URL+"/v1" || llm["model"] != "kept-model" {
		t.Fatalf("settings not restored: %#v", llm)
	}

	res = do(t, srv, http.MethodPost, "/api/models", map[string]string{
		"provider": "openai",
		"base_url": upstream.URL + "/v1",
		"api_key":  "••••1234",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("models %d %s", res.StatusCode, read(t, res))
	}
	var listed struct {
		Models []string `json:"models"`
	}
	mustDecode(t, res, &listed)
	if auth != "Bearer sk-test-1234" || len(listed.Models) != 2 || listed.Models[0] != "alpha" {
		t.Fatalf("auth %q models %#v", auth, listed.Models)
	}
	res = do(t, srv, http.MethodPost, "/api/models", map[string]string{
		"provider": "openai",
		"base_url": "http://169.254.169.254/v1",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("metadata %d %s", res.StatusCode, read(t, res))
	}

	now := time.Now().UTC()
	insert := func(id, title, desc string, min, max int) {
		t.Helper()
		job := model.Job{
			ID: id, SourceID: "src", SourceKind: "json", ExternalID: id, Fingerprint: id,
			Title: title, Company: "Co", URL: "https://example.com/" + id, Description: desc,
			SalaryMin: min, SalaryMax: max, Currency: "USD", Status: model.StatusMatched,
			FirstSeen: now, LastSeen: now, UpdatedAt: now,
		}
		if _, err := st.UpsertJob(job); err != nil {
			t.Fatal(err)
		}
	}
	insert("high", "High", "", 200000, 220000)
	insert("low", "Low", "", 80000, 90000)
	insert("cap", "Cap", "", 0, 150000)
	insert("none", "None", "No pay listed", 0, 0)
	insert("text", "From text", "Salary $90,000 - $110,000 per year", 0, 0)
	insert("dotted", "Dotted", "Hiring Ranges: Remote Germany €68.000 — €91.000 EUR", 68, 91)
	insert("gift", "Gift", "We match up to $2000 for donations.", 2000, 2000)
	n, err := st.BackfillSalaries()
	if err != nil || n != 3 {
		t.Fatalf("backfill %d %v", n, err)
	}

	assertOrder := func(sort string, want ...string) {
		t.Helper()
		res := do(t, srv, http.MethodGet, "/api/jobs?sort="+sort, nil)
		var list struct {
			Jobs []model.ListItem `json:"jobs"`
		}
		mustDecode(t, res, &list)
		got := make([]string, len(list.Jobs))
		for i, job := range list.Jobs {
			got[i] = job.Title
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("sort %s got %v want %v", sort, got, want)
		}
	}
	assertOrder("start_asc", "Dotted", "Low", "From text", "Cap", "High", "Gift", "None")
	assertOrder("start_desc", "High", "Cap", "From text", "Low", "Dotted", "Gift", "None")
	assertOrder("high_desc", "High", "Cap", "From text", "Dotted", "Low", "Gift", "None")
	assertOrder("high_asc", "Low", "Dotted", "From text", "Cap", "High", "Gift", "None")

	res = do(t, srv, http.MethodGet, "/api/jobs?source=src", nil)
	var filtered struct {
		Jobs []model.ListItem `json:"jobs"`
	}
	mustDecode(t, res, &filtered)
	if len(filtered.Jobs) != 7 {
		t.Fatalf("source src: %d", len(filtered.Jobs))
	}
	res = do(t, srv, http.MethodGet, "/api/jobs?source=other", nil)
	mustDecode(t, res, &filtered)
	if len(filtered.Jobs) != 0 {
		t.Fatalf("source other: %d", len(filtered.Jobs))
	}
	res = do(t, srv, http.MethodGet, "/api/jobs?source=src,not%20an%20id", nil)
	mustDecode(t, res, &filtered)
	if len(filtered.Jobs) != 7 {
		t.Fatalf("mixed source: %d", len(filtered.Jobs))
	}
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func mustDecode(t *testing.T, res *http.Response, dest any) {
	t.Helper()
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(dest); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

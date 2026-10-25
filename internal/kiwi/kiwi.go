// Package kiwi writes job pages to a local folder and, optionally, a KiwiFS workspace.
package kiwi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

// Document is one markdown page under the context prefix.
type Document struct {
	Rel     string
	Content string
}

// Result reports where pages were written.
type Result struct {
	LocalDir string
	Files    int
	Kiwi     string
}

// Context is the workspace briefing KiwiFS returns for agents.
type Context struct {
	Playbook string `json:"playbook"`
	Schema   string `json:"schema"`
	Index    string `json:"index"`
	Rules    string `json:"rules"`
}

// Writer stores pages locally and mirrors them when KiwiFS is enabled.
type Writer struct {
	LocalDir string
	Kiwi     model.KiwiConfig
	Client   *http.Client
}

// NewWriter uses dataDir/context as the local filesystem.
func NewWriter(dataDir string, cfg model.KiwiConfig) *Writer {
	return &Writer{
		LocalDir: filepath.Join(dataDir, "context"),
		Kiwi:     cfg,
		Client:   &http.Client{Timeout: 20 * time.Second},
	}
}

// Write stores every document. A KiwiFS failure does not roll back the local copies.
func (w *Writer) Write(ctx context.Context, docs []Document) (Result, error) {
	prefix, err := textutil.SafePathPrefix(w.Kiwi.PathPrefix)
	if err != nil {
		return Result{}, err
	}
	res := Result{LocalDir: w.LocalDir, Kiwi: "disabled"}
	var kiwiErr error
	for _, doc := range docs {
		rel, err := textutil.SafeRel(prefix, doc.Rel)
		if err != nil {
			return res, err
		}
		if err := writeLocal(w.LocalDir, rel, doc.Content); err != nil {
			return res, err
		}
		res.Files++
		if w.Kiwi.Enabled {
			if err := w.put(ctx, rel, doc.Content); err != nil && kiwiErr == nil {
				kiwiErr = err
			}
		}
	}
	if !w.Kiwi.Enabled {
		return res, nil
	}
	if kiwiErr != nil {
		res.Kiwi = kiwiErr.Error()
		return res, nil
	}
	res.Kiwi = "ok"
	return res, nil
}

// LoadContext reads the KiwiFS session context when a base URL is configured.
func (w *Writer) LoadContext(ctx context.Context) (Context, error) {
	if strings.TrimSpace(w.Kiwi.BaseURL) == "" {
		return Context{}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(w.Kiwi.BaseURL, "/")+"/api/kiwi/context", nil)
	if err != nil {
		return Context{}, err
	}
	w.decorate(req)
	res, err := w.Client.Do(req)
	if err != nil {
		return Context{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Context{}, fmt.Errorf("kiwi context %s", res.Status)
	}
	var out Context
	if err := json.Unmarshal(body, &out); err != nil {
		return Context{}, err
	}
	return out, nil
}

func (w *Writer) put(ctx context.Context, rel, content string) error {
	base := strings.TrimRight(strings.TrimSpace(w.Kiwi.BaseURL), "/")
	if base == "" {
		return fmt.Errorf("kiwi base url is empty")
	}
	endpoint := base + "/api/kiwi/file?path=" + url.QueryEscape(rel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader([]byte(content)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/markdown")
	w.decorate(req)
	res, err := w.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 180 {
			msg = msg[:180]
		}
		return fmt.Errorf("kiwi write %s: %s", res.Status, msg)
	}
	return nil
}

func (w *Writer) decorate(req *http.Request) {
	req.Header.Set("X-Actor", "agent:job-finder")
	if key := strings.TrimSpace(w.Kiwi.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if space := strings.TrimSpace(w.Kiwi.Space); space != "" {
		req.Header.Set("X-Kiwi-Space", space)
	}
}

func writeLocal(root, rel, content string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// Pages builds the markdown tree for a profile, a run, and the roles worth keeping.
func Pages(profile model.Profile, run *model.Run, jobs []model.Job) []Document {
	docs := []Document{
		{Rel: "index.md", Content: indexPage(profile, run, jobs)},
		{Rel: "profile.md", Content: profilePage(profile)},
		{Rel: "schema.md", Content: schemaPage()},
	}
	if run != nil {
		docs = append(docs, Document{Rel: "runs/" + run.ID + ".md", Content: runPage(*run)})
	}
	seenCompany := map[string]bool{}
	for _, job := range jobs {
		docs = append(docs, Document{Rel: "jobs/" + jobFile(job), Content: jobPage(job)})
		company := textutil.Slug(job.Company)
		if job.Research != "" && !seenCompany[company] {
			seenCompany[company] = true
			docs = append(docs, Document{Rel: "companies/" + company + ".md", Content: companyPage(job)})
		}
	}
	return docs
}

func indexPage(profile model.Profile, run *model.Run, jobs []model.Job) string {
	var b strings.Builder
	b.WriteString("---\ntype: index\nstatus: active\n---\n\n# Job search\n\n")
	b.WriteString("Requirements live in [profile](profile.md). Each matched or saved role is a page under `jobs/`.\n\n")
	if run != nil {
		fmt.Fprintf(&b, "Latest run `%s` (%s): fetched %d, new %d, matched %d, rejected %d.\n\n",
			run.ID, run.Status, run.Fetched, run.New, run.Matched, run.Rejected)
	}
	if len(profile.Keywords) > 0 {
		fmt.Fprintf(&b, "Keywords: %s.\n\n", strings.Join(profile.Keywords, ", "))
	}
	b.WriteString("## Roles\n\n")
	if len(jobs) == 0 {
		b.WriteString("No matched or saved roles yet.\n")
		return b.String()
	}
	for _, job := range jobs {
		score := "unscored"
		if job.Score >= 0 {
			score = fmt.Sprintf("%d", job.Score)
		}
		fmt.Fprintf(&b, "- [%s — %s](jobs/%s) · %s · score %s\n", job.Title, job.Company, jobFile(job), job.Status, score)
	}
	return b.String()
}

func profilePage(p model.Profile) string {
	var b strings.Builder
	b.WriteString("---\ntype: profile\n---\n\n# Search profile\n\n")
	fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(p.Notes))
	writeList(&b, "Keywords", p.Keywords)
	writeList(&b, "Must have", p.MustHave)
	writeList(&b, "Exclude", p.Exclude)
	writeList(&b, "Locations", p.Locations)
	fmt.Fprintf(&b, "\nRemote only: %t. Include remote when a location is set: %t.\n\nMinimum pay: %d. Minimum model score: %d.\n",
		p.RemoteOnly, p.IncludeRemote, p.MinSalary, p.MinScore)
	return b.String()
}

func writeList(b *strings.Builder, title string, items []string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(items) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", item)
	}
	b.WriteString("\n")
}

func schemaPage() string {
	return `---
type: schema
---

# Job page fields

These pages are a context pack for an agent. They do not replace a workspace SCHEMA.md.

- type: job, profile, run, company, or index
- status: matched or saved
- score: 0-100 from the model, or -1 when the model did not score the role
- source: adapter that found the posting
- url: original posting
- remote: true when the source or the location text says remote
- salary_min and salary_max: numbers as published, without currency conversion
`
}

func runPage(run model.Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntype: run\nstatus: %s\ntrigger: %s\n---\n\n# Run %s\n\n", run.Status, run.Trigger, run.ID)
	fmt.Fprintf(&b, "Started %s.\n\nFetched %d, new %d, matched %d, rejected %d.\n\n%s\n",
		run.StartedAt.Format(time.RFC3339), run.Fetched, run.New, run.Matched, run.Rejected, run.Message)
	return b.String()
}

func jobPage(job model.Job) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntype: job\ntitle: %q\ncompany: %q\nstatus: %s\nscore: %d\nsource: %s\nurl: %q\nlocation: %q\nremote: %t\nsalary_min: %d\nsalary_max: %d\n---\n\n# %s\n\n%s · %s\n\n",
		oneLine(job.Title), oneLine(job.Company), job.Status, job.Score, job.SourceKind, oneLine(job.URL), oneLine(job.Location), job.Remote, job.SalaryMin, job.SalaryMax,
		job.Title, job.Company, blank(job.Location, "Location unknown"))
	if len(job.FitPros) > 0 || len(job.FitCons) > 0 {
		b.WriteString("## Fit\n\n")
		for _, item := range job.FitPros {
			fmt.Fprintf(&b, "+ %s\n", item)
		}
		for _, item := range job.FitCons {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	} else if job.FitSummary != "" {
		fmt.Fprintf(&b, "## Fit\n\n%s\n\n", job.FitSummary)
	}
	if job.FilterReason != "" {
		fmt.Fprintf(&b, "Rule note: %s\n\n", job.FilterReason)
	}
	b.WriteString("## Posting\n\n")
	b.WriteString(blank(job.Description, "No description was published."))
	b.WriteString("\n\n")
	if job.Research != "" {
		fmt.Fprintf(&b, "## Research\n\n%s\n", job.Research)
	}
	if job.URL != "" {
		fmt.Fprintf(&b, "\n[Open posting](%s)\n", job.URL)
	}
	return b.String()
}

func companyPage(job model.Job) string {
	return fmt.Sprintf("---\ntype: company\ncompany: %q\n---\n\n# %s\n\n%s\n", oneLine(job.Company), job.Company, job.Research)
}

func jobFile(job model.Job) string {
	return textutil.Slug(job.Company) + "-" + textutil.Slug(job.Title) + "-" + job.ID + ".md"
}

// PruneJobPages removes local role pages whose ids are not in keep.
func PruneJobPages(root, prefix string, keep map[string]bool) (int, error) {
	prefix, err := textutil.SafePathPrefix(prefix)
	if err != nil {
		return 0, err
	}
	base := filepath.Clean(root)
	dir := filepath.Clean(filepath.Join(base, filepath.FromSlash(prefix), "jobs"))
	if dir != base && !strings.HasPrefix(dir, base+string(filepath.Separator)) {
		return 0, fmt.Errorf("context path escapes the folder")
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		id := jobIDFromPage(name)
		if id == "" || keep[id] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func jobIDFromPage(name string) string {
	base := strings.TrimSuffix(name, ".md")
	cut := strings.LastIndex(base, "-")
	if cut < 0 || cut == len(base)-1 {
		return ""
	}
	id := base[cut+1:]
	if !strings.HasPrefix(id, "job_") {
		return ""
	}
	return id
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func blank(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

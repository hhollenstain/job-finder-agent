// Package agent runs a search: fetch, filter, optionally score and research, then write context.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/filter"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/source"
	"github.com/henryhollenstain/job-finder-agent/internal/store"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

// ErrBusy is returned when a search is already in progress.
var ErrBusy = errors.New("a search is already running")

// Agent coordinates sources, the model, and the context filesystem.
type Agent struct {
	store   *store.Store
	dataDir string

	mu      sync.Mutex
	running bool
}

// New returns an agent bound to the local database.
func New(st *store.Store, dataDir string) *Agent {
	return &Agent{store: st, dataDir: dataDir}
}

// Start records a run and executes it in the background.
func (a *Agent) Start(trigger string, force bool) (model.Run, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return model.Run{}, ErrBusy
	}
	now := time.Now().UTC()
	run := model.Run{
		ID:        newID("run_"),
		Trigger:   trigger,
		Status:    model.RunRunning,
		StartedAt: now,
		Message:   "starting",
	}
	if err := a.store.InsertRun(run); err != nil {
		return model.Run{}, err
	}
	a.running = true
	go func() {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		defer func() {
			if rec := recover(); rec != nil {
				run.Status = model.RunError
				run.Message = fmt.Sprintf("panic: %v", rec)
				done := time.Now().UTC()
				run.FinishedAt = &done
				_ = a.store.UpdateRun(run)
			}
		}()
		a.execute(ctx, &run, force)
	}()
	return run, nil
}

func (a *Agent) execute(ctx context.Context, run *model.Run, force bool) {
	profile, err := a.store.Profile()
	if err != nil {
		a.finish(run, model.RunError, err.Error())
		return
	}
	settings, err := a.store.Settings()
	if err != nil {
		a.finish(run, model.RunError, err.Error())
		return
	}
	settings = normalizeSettings(settings)
	sources, err := a.store.ListSources()
	if err != nil {
		a.finish(run, model.RunError, err.Error())
		return
	}
	var notes []string
	drafts := a.fetchAll(ctx, sources, profile, settings, force, &notes)
	if settings.ExpandSources {
		added := a.expand(ctx, settings, profile, sources, len(passing(drafts, profile)))
		if len(added) > 0 {
			names := make([]string, 0, len(added))
			for _, src := range added {
				names = append(names, src.Name)
			}
			notes = append(notes, "added sources: "+strings.Join(names, ", "))
			more := a.fetchAll(ctx, added, profile, settings, true, &notes)
			drafts = append(drafts, more...)
		}
	}
	run.Fetched = len(drafts)
	run.Message = "filtering " + fmt.Sprint(len(drafts))
	_ = a.store.UpdateRun(*run)

	cands := make([]candidate, 0, len(drafts))
	for _, item := range drafts {
		c := candidate{draft: item.draft, source: item.source, score: -1}
		job := c.preview()
		decision := filter.Decide(profile, job)
		c.pass = decision.Pass
		c.reason = decision.Reason
		if decision.Pass {
			c.status = model.StatusMatched
			c.pros, c.cons = filter.Explain(profile, job)
			c.summary = filter.FormatFit(c.pros, c.cons)
		} else {
			c.status = model.StatusRejected
		}
		cands = append(cands, c)
	}

	if settings.ScoreWithLLM {
		if err := a.score(ctx, settings, profile, cands); err != nil {
			notes = append(notes, "scoring skipped: "+err.Error())
		}
	}

	now := time.Now().UTC()
	for i := range cands {
		if strings.TrimSpace(cands[i].draft.Title) == "" {
			continue
		}
		job := cands[i].toJob(now)
		res, err := a.store.UpsertJob(job)
		if err != nil {
			notes = append(notes, "save "+job.Title+": "+err.Error())
			continue
		}
		if res.DuplicateOf != "" {
			continue
		}
		if res.Created {
			run.New++
		}
		switch cands[i].status {
		case model.StatusMatched, model.StatusSaved:
			run.Matched++
		case model.StatusRejected:
			run.Rejected++
		}
	}

	if settings.AutoResearch && settings.ScoreWithLLM {
		if err := a.autoResearch(ctx, settings, profile); err != nil {
			notes = append(notes, "research: "+err.Error())
		}
	}

	if len(notes) == 0 {
		notes = append(notes, "finished")
	}
	a.finish(run, model.RunOK, strings.Join(notes, "\n"))
	if _, err := a.Sync(ctx); err != nil {
		run.Message = textutil.Truncate(run.Message+"\ncontext: "+err.Error(), 4000)
		_ = a.store.UpdateRun(*run)
	}
}

type fetched struct {
	source model.Source
	draft  model.JobDraft
}

func (a *Agent) fetchAll(ctx context.Context, sources []model.Source, profile model.Profile, settings model.Settings, force bool, notes *[]string) []fetched {
	var (
		mu  sync.Mutex
		out []fetched
		wg  sync.WaitGroup
		sem = make(chan struct{}, 4)
	)
	ttl := time.Duration(settings.CacheHours) * time.Hour
	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		wg.Add(1)
		go func(src model.Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			drafts, err := a.fetchOne(ctx, src, profile, ttl, force)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				*notes = append(*notes, src.Name+": "+err.Error())
				return
			}
			for _, d := range drafts {
				out = append(out, fetched{source: src, draft: d})
			}
		}(src)
	}
	wg.Wait()
	return out
}

func (a *Agent) fetchOne(ctx context.Context, src model.Source, profile model.Profile, ttl time.Duration, force bool) ([]model.JobDraft, error) {
	if !force && ttl > 0 {
		if cached, ok, err := a.store.CacheGet(src.ID, ttl); err != nil {
			return nil, err
		} else if ok {
			return cached, nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	drafts, err := source.Search(ctx, src, source.Query{Profile: profile, Limit: 40})
	if err != nil {
		return nil, err
	}
	if err := a.store.CachePut(src.ID, drafts); err != nil {
		return drafts, nil
	}
	return drafts, nil
}

func passing(items []fetched, profile model.Profile) []fetched {
	var out []fetched
	for _, item := range items {
		job := candidate{draft: item.draft, source: item.source}.preview()
		if filter.Decide(profile, job).Pass {
			out = append(out, item)
		}
	}
	return out
}

func (a *Agent) finish(run *model.Run, status, message string) {
	run.Status = status
	run.Message = textutil.Truncate(message, 4000)
	done := time.Now().UTC()
	run.FinishedAt = &done
	_ = a.store.UpdateRun(*run)
}

type candidate struct {
	draft   model.JobDraft
	source  model.Source
	pass    bool
	reason  string
	score   int
	summary string
	pros    []string
	cons    []string
	status  string
}

func (c candidate) key() string {
	id := c.draft.ExternalID
	if id == "" {
		id = textutil.Hash(c.draft.URL, c.draft.Title, c.draft.Company)
	}
	return c.source.ID + ":" + id
}

func (c candidate) preview() model.Job {
	return c.toJob(time.Time{})
}

func (c candidate) toJob(now time.Time) model.Job {
	d := c.draft
	remote := textutil.LooksRemote(d.Location)
	if d.Remote != nil && *d.Remote {
		remote = true
	}
	company := strings.TrimSpace(d.Company)
	if company == "" {
		company = "Unknown"
	}
	title := strings.TrimSpace(d.Title)
	ext := d.ExternalID
	if ext == "" {
		ext = textutil.Hash(d.URL, title, company)
	}
	status := c.status
	if status == "" {
		status = model.StatusNew
	}
	desc := textutil.Truncate(textutil.StripHTML(d.Description), 20000)
	min, max, cur := d.SalaryMin, d.SalaryMax, strings.TrimSpace(d.Currency)
	if min == 0 && max == 0 {
		if parsedMin, parsedMax, parsedCur, ok := textutil.ParseSalary(title + "\n" + desc); ok {
			min, max = parsedMin, parsedMax
			if cur == "" {
				cur = parsedCur
			}
		}
	}
	return model.Job{
		ID:             newID("job_"),
		SourceID:       c.source.ID,
		SourceKind:     c.source.Kind,
		ExternalID:     ext,
		Fingerprint:    textutil.Fingerprint(company, title, d.Location, d.URL),
		Title:          title,
		Company:        company,
		Location:       strings.TrimSpace(d.Location),
		Remote:         remote,
		URL:            strings.TrimSpace(d.URL),
		Description:    desc,
		SalaryMin:      min,
		SalaryMax:      max,
		Currency:       cur,
		EmploymentType: d.EmploymentType,
		PostedAt:       d.PostedAt,
		Status:         status,
		Score:          c.score,
		FilterReason:   c.reason,
		FitSummary:     textutil.Truncate(c.summary, 800),
		FitPros:        c.pros,
		FitCons:        c.cons,
		FirstSeen:      now,
		LastSeen:       now,
		UpdatedAt:      now,
	}
}

func newID(prefix string) string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return prefix + hex.EncodeToString(buf)
}

func normalizeSettings(s model.Settings) model.Settings {
	if s.MaxLLMJobs <= 0 {
		s.MaxLLMJobs = 25
	}
	if s.CacheHours < 0 {
		s.CacheHours = 6
	}
	if strings.TrimSpace(s.Kiwi.PathPrefix) == "" {
		s.Kiwi.PathPrefix = "job-search"
	}
	if strings.TrimSpace(s.Timezone) == "" {
		s.Timezone = "Local"
	}
	if strings.TrimSpace(s.LLM.Provider) == "" {
		s.LLM.Provider = "ollama"
	}
	return s
}

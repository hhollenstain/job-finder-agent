package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/llm"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/source"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

// ResearchJob reads the stored posting, fetches the public page when there is one, and asks the model for a briefing.
func (a *Agent) ResearchJob(ctx context.Context, id string) (model.Job, error) {
	job, err := a.store.GetJob(id)
	if err != nil {
		return model.Job{}, err
	}
	settings, err := a.store.Settings()
	if err != nil {
		return model.Job{}, err
	}
	profile, err := a.store.Profile()
	if err != nil {
		return model.Job{}, err
	}
	provider, err := llm.New(normalizeSettings(settings).LLM)
	if err != nil {
		return model.Job{}, err
	}
	page := ""
	if job.URL != "" {
		page = fetchPublicPage(ctx, job.URL)
	}
	system := `You brief a reader who is deciding whether to apply. Separate what the posting says from general knowledge. Start general-knowledge sentences with "Outside the posting:". Do not claim the company is hiring for anything the text does not support. Plain text with these headings:

Company
Role
Requirements check
Risks
Questions to ask`
	user := fmt.Sprintf("Profile:\n%s\n\nMust have: %s\nKeywords: %s\n\nRole: %s\nCompany: %s\nLocation: %s\nURL: %s\n\nStored description:\n%s\n\nFetched page text:\n%s\n",
		profile.Notes, strings.Join(profile.MustHave, ", "), strings.Join(profile.Keywords, ", "),
		job.Title, job.Company, job.Location, job.URL,
		textutil.Truncate(job.Description, 6000), textutil.Truncate(page, 6000))
	if strings.TrimSpace(page) == "" {
		user += "\nThe public page could not be fetched. Use the stored description only, and say that.\n"
	}
	text, err := provider.Complete(ctx, system, user)
	if err != nil {
		return model.Job{}, err
	}
	text = textutil.Truncate(strings.TrimSpace(text), 12000)
	if err := a.store.SaveResearch(id, text); err != nil {
		return model.Job{}, err
	}
	job.Research = text
	return job, nil
}

func (a *Agent) autoResearch(ctx context.Context, settings model.Settings, profile model.Profile) error {
	jobs, err := a.store.JobsForContext(5)
	if err != nil {
		return err
	}
	_ = settings
	_ = profile
	n := 0
	for _, job := range jobs {
		if job.Research != "" {
			continue
		}
		if _, err := a.ResearchJob(ctx, job.ID); err != nil {
			return err
		}
		n++
		if n >= 5 {
			break
		}
	}
	return nil
}

func fetchPublicPage(ctx context.Context, rawURL string) string {
	if err := source.CheckURL(rawURL); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "JobFinderAgent/0.1 (personal job search)")
	req.Header.Set("Accept", "text/html, text/plain")
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ""
	}
	ctype := res.Header.Get("Content-Type")
	if ctype != "" && !strings.Contains(ctype, "text/") && !strings.Contains(ctype, "xml") && !strings.Contains(ctype, "json") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return ""
	}
	return textutil.StripHTML(string(body))
}

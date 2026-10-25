package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

func searchRemotive(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	u := "https://remotive.com/api/remote-jobs?limit=" + fmt.Sprint(q.Limit)
	if term := searchTerms(cfg, q); term != "" {
		u += "&search=" + queryEscape(term)
	}
	var payload struct {
		Jobs []struct {
			ID          FlexString `json:"id"`
			URL         string     `json:"url"`
			Title       string     `json:"title"`
			Company     string     `json:"company_name"`
			Category    string     `json:"category"`
			JobType     string     `json:"job_type"`
			Publication string     `json:"publication_date"`
			Location    string     `json:"candidate_required_location"`
			Salary      string     `json:"salary"`
			Description string     `json:"description"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, u, nil, &payload); err != nil {
		return nil, err
	}
	remote := true
	out := make([]model.JobDraft, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		if strings.TrimSpace(job.Title) == "" {
			continue
		}
		out = append(out, model.JobDraft{
			ExternalID:     job.ID.String(),
			Title:          job.Title,
			Company:        job.Company,
			Location:       job.Location,
			Remote:         &remote,
			URL:            job.URL,
			Description:    textutil.StripHTML(job.Description + "\n" + job.Salary),
			EmploymentType: job.JobType,
			PostedAt:       textutil.ParseTime(job.Publication),
		})
	}
	return out, nil
}

func searchRemoteOK(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	body, err := getBytes(ctx, "https://remoteok.com/api", map[string]string{"Accept": "application/json"})
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode remote ok: %w", err)
	}
	term := strings.ToLower(searchTerms(cfg, q))
	remote := true
	out := make([]model.JobDraft, 0, len(raw))
	for _, item := range raw {
		var job struct {
			ID          FlexString `json:"id"`
			Position    string     `json:"position"`
			Company     string     `json:"company"`
			Description string     `json:"description"`
			URL         string     `json:"url"`
			ApplyURL    string     `json:"apply_url"`
			Location    string     `json:"location"`
			Date        string     `json:"date"`
			Epoch       FlexString `json:"epoch"`
			SalaryMin   FlexInt    `json:"salary_min"`
			SalaryMax   FlexInt    `json:"salary_max"`
			Tags        []string   `json:"tags"`
		}
		if err := json.Unmarshal(item, &job); err != nil || job.Position == "" {
			continue
		}
		desc := textutil.StripHTML(job.Description)
		if term != "" && !strings.Contains(strings.ToLower(job.Position+" "+job.Company+" "+desc+" "+strings.Join(job.Tags, " ")), term) {
			// A multi-word query matches if any word hits. The hard filter still runs later.
			hit := false
			for _, word := range strings.Fields(term) {
				if len(word) > 2 && strings.Contains(strings.ToLower(job.Position+" "+desc+" "+strings.Join(job.Tags, " ")), word) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		link := job.URL
		if link == "" {
			link = job.ApplyURL
		}
		when := job.Date
		if when == "" {
			when = job.Epoch.String()
		}
		loc := job.Location
		if loc == "" {
			loc = "Remote"
		}
		out = append(out, model.JobDraft{
			ExternalID:  job.ID.String(),
			Title:       job.Position,
			Company:     job.Company,
			Location:    loc,
			Remote:      &remote,
			URL:         link,
			Description: desc,
			SalaryMin:   int(job.SalaryMin),
			SalaryMax:   int(job.SalaryMax),
			Currency:    "USD",
			PostedAt:    textutil.ParseTime(when),
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func searchArbeitnow(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	var payload struct {
		Data []struct {
			Slug        string     `json:"slug"`
			Company     string     `json:"company_name"`
			Title       string     `json:"title"`
			Description string     `json:"description"`
			Remote      bool       `json:"remote"`
			URL         string     `json:"url"`
			Location    string     `json:"location"`
			Created     FlexString `json:"created_at"`
			JobTypes    []string   `json:"job_types"`
			Tags        []string   `json:"tags"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://www.arbeitnow.com/api/job-board-api", nil, &payload); err != nil {
		return nil, err
	}
	term := strings.ToLower(searchTerms(cfg, q))
	out := make([]model.JobDraft, 0, len(payload.Data))
	for _, job := range payload.Data {
		desc := textutil.StripHTML(job.Description)
		blob := strings.ToLower(job.Title + " " + job.Company + " " + desc + " " + strings.Join(job.Tags, " "))
		if term != "" && !containsAnyWord(blob, term) {
			continue
		}
		remote := job.Remote
		kind := ""
		if len(job.JobTypes) > 0 {
			kind = job.JobTypes[0]
		}
		out = append(out, model.JobDraft{
			ExternalID:     job.Slug,
			Title:          job.Title,
			Company:        job.Company,
			Location:       job.Location,
			Remote:         &remote,
			URL:            job.URL,
			Description:    desc,
			EmploymentType: kind,
			PostedAt:       textutil.ParseTime(job.Created.String()),
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func containsAnyWord(blob, term string) bool {
	if strings.Contains(blob, term) {
		return true
	}
	for _, word := range strings.Fields(term) {
		if len(word) > 2 && strings.Contains(blob, word) {
			return true
		}
	}
	return false
}

func searchAdzuna(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	country := cfgOr(cfg, "country", "us")
	params := url.Values{}
	params.Set("app_id", cfg["app_id"])
	params.Set("app_key", cfg["app_key"])
	params.Set("results_per_page", fmt.Sprint(min(q.Limit, 50)))
	params.Set("content-type", "application/json")
	if what := searchTerms(cfg, q); what != "" {
		params.Set("what", what)
	}
	if where := strings.TrimSpace(cfg["where"]); where != "" {
		params.Set("where", where)
	}
	u := fmt.Sprintf("https://api.adzuna.com/v1/api/jobs/%s/search/1?%s", url.PathEscape(country), params.Encode())
	var payload struct {
		Results []struct {
			ID          FlexString `json:"id"`
			Title       string     `json:"title"`
			Description string     `json:"description"`
			Redirect    string     `json:"redirect_url"`
			Created     string     `json:"created"`
			SalaryMin   FlexInt    `json:"salary_min"`
			SalaryMax   FlexInt    `json:"salary_max"`
			Company     struct {
				Name string `json:"display_name"`
			} `json:"company"`
			Location struct {
				Name string `json:"display_name"`
			} `json:"location"`
		} `json:"results"`
	}
	if err := getJSON(ctx, u, nil, &payload); err != nil {
		return nil, err
	}
	out := make([]model.JobDraft, 0, len(payload.Results))
	for _, job := range payload.Results {
		out = append(out, model.JobDraft{
			ExternalID:  job.ID.String(),
			Title:       job.Title,
			Company:     job.Company.Name,
			Location:    job.Location.Name,
			URL:         job.Redirect,
			Description: textutil.StripHTML(job.Description),
			SalaryMin:   int(job.SalaryMin),
			SalaryMax:   int(job.SalaryMax),
			PostedAt:    textutil.ParseTime(job.Created),
		})
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

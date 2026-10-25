package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

func searchGreenhouse(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	board := strings.TrimSpace(cfg["board"])
	u := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", board)
	var payload struct {
		Jobs []struct {
			ID       FlexString `json:"id"`
			Title    string     `json:"title"`
			URL      string     `json:"absolute_url"`
			Content  string     `json:"content"`
			Updated  string     `json:"updated_at"`
			Location struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, u, nil, &payload); err != nil {
		return nil, err
	}
	company := companyOr(cfg, board)
	out := make([]model.JobDraft, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		out = append(out, model.JobDraft{
			ExternalID:  board + ":" + job.ID.String(),
			Title:       job.Title,
			Company:     company,
			Location:    job.Location.Name,
			URL:         job.URL,
			Description: textutil.StripHTML(job.Content),
			PostedAt:    textutil.ParseTime(job.Updated),
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func searchLever(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	site := strings.TrimSpace(cfg["site"])
	u := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", site)
	var payload []struct {
		ID          string  `json:"id"`
		Text        string  `json:"text"`
		HostedURL   string  `json:"hostedUrl"`
		Description string  `json:"descriptionPlain"`
		CreatedAt   FlexInt `json:"createdAt"`
		Categories  struct {
			Location   string `json:"location"`
			Commitment string `json:"commitment"`
		} `json:"categories"`
	}
	if err := getJSON(ctx, u, nil, &payload); err != nil {
		return nil, err
	}
	company := companyOr(cfg, site)
	out := make([]model.JobDraft, 0, len(payload))
	for _, job := range payload {
		out = append(out, model.JobDraft{
			ExternalID:     site + ":" + job.ID,
			Title:          job.Text,
			Company:        company,
			Location:       job.Categories.Location,
			URL:            job.HostedURL,
			Description:    textutil.StripHTML(job.Description),
			EmploymentType: job.Categories.Commitment,
			PostedAt:       textutil.ParseTime(job.CreatedAt.StringTime()),
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func (f FlexInt) StringTime() string {
	if f <= 0 {
		return ""
	}
	return fmt.Sprint(int64(f))
}

func searchAshby(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	board := strings.TrimSpace(cfg["board"])
	u := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=true", board)
	var payload struct {
		Jobs []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Location    string `json:"location"`
			Type        string `json:"employmentType"`
			Description string `json:"descriptionHtml"`
			URL         string `json:"jobUrl"`
			IsRemote    bool   `json:"isRemote"`
			Published   string `json:"publishedAt"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, u, nil, &payload); err != nil {
		return nil, err
	}
	company := companyOr(cfg, board)
	out := make([]model.JobDraft, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		remote := job.IsRemote
		out = append(out, model.JobDraft{
			ExternalID:     board + ":" + job.ID,
			Title:          job.Title,
			Company:        company,
			Location:       job.Location,
			Remote:         &remote,
			URL:            job.URL,
			Description:    textutil.StripHTML(job.Description),
			EmploymentType: job.Type,
			PostedAt:       textutil.ParseTime(job.Published),
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

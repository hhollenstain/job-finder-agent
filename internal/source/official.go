package source

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

func searchUSAJobs(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	params := url.Values{}
	keyword := searchTerms(cfg, q)
	if keyword == "" {
		keyword = "software"
	}
	params.Set("Keyword", keyword)
	params.Set("ResultsPerPage", fmt.Sprint(min(q.Limit, 50)))
	if loc := strings.TrimSpace(cfg["location"]); loc != "" {
		params.Set("LocationName", loc)
	}
	u := "https://data.usajobs.gov/api/search?" + params.Encode()
	var payload struct {
		SearchResult struct {
			Items []struct {
				ID   string `json:"MatchedObjectId"`
				Desc struct {
					Title        string `json:"PositionTitle"`
					Org          string `json:"OrganizationName"`
					URI          string `json:"PositionURI"`
					Location     string `json:"PositionLocationDisplay"`
					Remuneration []struct {
						Min      string `json:"MinimumRange"`
						Max      string `json:"MaximumRange"`
						Currency string `json:"CurrencyCode"`
					} `json:"PositionRemuneration"`
					Schedule []struct {
						Name string `json:"Name"`
					} `json:"PositionSchedule"`
					Publication string `json:"PublicationStartDate"`
					UserArea    struct {
						Details struct {
							Summary string `json:"JobSummary"`
						} `json:"Details"`
					} `json:"UserArea"`
				} `json:"MatchedObjectDescriptor"`
			} `json:"SearchResultItems"`
		} `json:"SearchResult"`
	}
	headers := map[string]string{
		"Authorization-Key": strings.TrimSpace(cfg["api_key"]),
		"User-Agent":        strings.TrimSpace(cfg["email"]),
		"Host":              "data.usajobs.gov",
	}
	if err := getJSON(ctx, u, headers, &payload); err != nil {
		return nil, err
	}
	out := make([]model.JobDraft, 0, len(payload.SearchResult.Items))
	for _, item := range payload.SearchResult.Items {
		d := item.Desc
		draft := model.JobDraft{
			ExternalID:  item.ID,
			Title:       d.Title,
			Company:     d.Org,
			Location:    d.Location,
			URL:         d.URI,
			Description: textutil.StripHTML(d.UserArea.Details.Summary),
			PostedAt:    textutil.ParseTime(d.Publication),
		}
		if len(d.Remuneration) > 0 {
			draft.SalaryMin = int(parseFlex(d.Remuneration[0].Min))
			draft.SalaryMax = int(parseFlex(d.Remuneration[0].Max))
			draft.Currency = d.Remuneration[0].Currency
		}
		if len(d.Schedule) > 0 {
			draft.EmploymentType = d.Schedule[0].Name
		}
		out = append(out, draft)
	}
	return out, nil
}

func parseFlex(s string) FlexInt {
	var n FlexInt
	_ = n.UnmarshalJSON([]byte(strconvQuote(s)))
	return n
}

func strconvQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, "") + `"`
}

func searchJSearch(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	query := searchTerms(cfg, q)
	if query == "" {
		query = "software engineer"
	}
	u := "https://jsearch.p.rapidapi.com/search?page=1&num_pages=1&query=" + queryEscape(query)
	var payload struct {
		Data []struct {
			ID          string  `json:"job_id"`
			Title       string  `json:"job_title"`
			Employer    string  `json:"employer_name"`
			Publisher   string  `json:"job_publisher"`
			Apply       string  `json:"job_apply_link"`
			Description string  `json:"job_description"`
			City        string  `json:"job_city"`
			State       string  `json:"job_state"`
			Country     string  `json:"job_country"`
			Remote      bool    `json:"job_is_remote"`
			Min         FlexInt `json:"job_min_salary"`
			Max         FlexInt `json:"job_max_salary"`
			Currency    string  `json:"job_salary_currency"`
			Type        string  `json:"job_employment_type"`
			Posted      string  `json:"job_posted_at_datetime_utc"`
		} `json:"data"`
	}
	headers := map[string]string{
		"X-RapidAPI-Key":  strings.TrimSpace(cfg["api_key"]),
		"X-RapidAPI-Host": "jsearch.p.rapidapi.com",
	}
	if err := getJSON(ctx, u, headers, &payload); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, name := range splitComma(cfg["publishers"]) {
		want[strings.ToLower(name)] = true
	}
	out := make([]model.JobDraft, 0, len(payload.Data))
	for _, job := range payload.Data {
		if len(want) > 0 && !want[strings.ToLower(job.Publisher)] {
			continue
		}
		remote := job.Remote
		loc := joinComma(job.City, job.State, job.Country)
		if remote && !strings.Contains(strings.ToLower(loc), "remote") {
			if loc == "" {
				loc = "Remote"
			} else {
				loc = "Remote · " + loc
			}
		}
		out = append(out, model.JobDraft{
			ExternalID:     job.ID,
			Title:          job.Title,
			Company:        job.Employer,
			Location:       loc,
			Remote:         &remote,
			URL:            job.Apply,
			Description:    textutil.StripHTML(job.Description + "\nPublisher: " + job.Publisher),
			SalaryMin:      int(job.Min),
			SalaryMax:      int(job.Max),
			Currency:       job.Currency,
			EmploymentType: job.Type,
			PostedAt:       textutil.ParseTime(job.Posted),
		})
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func joinComma(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

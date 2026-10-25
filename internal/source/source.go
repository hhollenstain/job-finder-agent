// Package source fetches postings from job boards, feeds, and Slack channels.
package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

// Query is the slice of the profile a source can push down to its API.
type Query struct {
	Profile model.Profile
	Limit   int
}

// Search runs the adapter for src.
func Search(ctx context.Context, src model.Source, q Query) ([]model.JobDraft, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	switch src.Kind {
	case "remotive":
		return searchRemotive(ctx, src.Config, q)
	case "remoteok":
		return searchRemoteOK(ctx, src.Config, q)
	case "arbeitnow":
		return searchArbeitnow(ctx, src.Config, q)
	case "adzuna":
		return searchAdzuna(ctx, src.Config, q)
	case "usajobs":
		return searchUSAJobs(ctx, src.Config, q)
	case "linkedin":
		return searchLinkedIn(ctx, src.Config, q)
	case "jsearch":
		return searchJSearch(ctx, src.Config, q)
	case "greenhouse":
		return searchGreenhouse(ctx, src.Config, q)
	case "lever":
		return searchLever(ctx, src.Config, q)
	case "ashby":
		return searchAshby(ctx, src.Config, q)
	case "rss":
		return searchRSS(ctx, src.Config, q)
	case "slack":
		return searchSlack(ctx, src.Config, q)
	case "json":
		return searchJSON(ctx, src.Config, q)
	default:
		return nil, fmt.Errorf("unknown source kind %q", src.Kind)
	}
}

func searchTerms(cfg map[string]string, q Query) string {
	if v := strings.TrimSpace(cfg["query"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(cfg["what"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(cfg["keyword"]); v != "" {
		return v
	}
	if len(q.Profile.Keywords) > 0 {
		return strings.Join(q.Profile.Keywords, " ")
	}
	return ""
}

func limitDrafts(in []model.JobDraft, n int) []model.JobDraft {
	if n <= 0 || len(in) <= n {
		return in
	}
	return in[:n]
}

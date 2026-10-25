package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/filter"
	"github.com/henryhollenstain/job-finder-agent/internal/llm"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

func (a *Agent) score(ctx context.Context, settings model.Settings, profile model.Profile, cands []candidate) error {
	provider, err := llm.New(settings.LLM)
	if err != nil {
		return err
	}
	var indexes []int
	for i := range cands {
		if cands[i].pass {
			indexes = append(indexes, i)
		}
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		return rankCandidate(cands[indexes[i]]) > rankCandidate(cands[indexes[j]])
	})
	if len(indexes) > settings.MaxLLMJobs {
		indexes = indexes[:settings.MaxLLMJobs]
	}
	if len(indexes) == 0 {
		return nil
	}
	extra := ""
	if settings.Kiwi.BaseURL != "" {
		if pack, err := a.kiwiContext(ctx, settings); err == nil {
			extra = pack
		}
	}
	system := `You screen jobs for one person. Use only the profile, the optional workspace notes, and the job text. Do not invent employers, salaries, locations, or requirements that are not in the text. If a posting is thin, say so in a con. Do not use that as a reason to flatten every score.

Return a JSON array and nothing else. Each element is {"id":"...","score":0,"pros":["short clause"],"cons":["short clause"]}.
Give 2 to 5 pros and 2 to 5 cons. Each item is one short clause with no leading plus or minus. Ground every item in the posting or the profile.

Score the role, from 0 to 100, on this scale. Spread the scores. Do not park a real engineering match at 35.
- 85 to 100: senior infrastructure, platform, DevOps, SRE, or distributed systems engineering, with stack overlap. Seattle, remote, or a US-wide posting. Listed pay reaches the minimum, or pay is unlisted.
- 70 to 84: an engineering role in that family with partial overlap, or listed pay whose top is at least about 200000 but short of the preferred number.
- 40 to 69: adjacent engineering, a thin posting, or pay clearly under 200000.
- 0 to 39: not an engineering role, such as sales, design, finance, recruiting, or support.
A listed range whose top is 200000 or more is a plus. Unknown pay is not a reason to score a fitting engineering role under 50. A senior infrastructure engineer in Seattle working with Kubernetes, pay listed around 200000 to 260000, is about 85. A graphic designer or account manager is under 20.`
	if extra != "" {
		system += "\n\nWorkspace notes, for extra context. The profile is the search contract when they disagree.\n" + extra
	}

	const batch = 6
	for start := 0; start < len(indexes); start += batch {
		end := start + batch
		if end > len(indexes) {
			end = len(indexes)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Profile notes:\n%s\n\nKeywords: %s\nMust have: %s\nExclude: %s\nLocations: %s\nRemote only: %t\nMinimum pay: %d\n\nJobs:\n",
			profile.Notes, strings.Join(profile.Keywords, ", "), strings.Join(profile.MustHave, ", "),
			strings.Join(profile.Exclude, ", "), strings.Join(profile.Locations, ", "), profile.RemoteOnly, profile.MinSalary)
		for _, idx := range indexes[start:end] {
			c := cands[idx]
			fmt.Fprintf(&b, "\n---\nid: %s\ntitle: %s\ncompany: %s\nlocation: %s\nremote: %t\npay: %d-%d %s\ntype: %s\ndescription:\n%s\n",
				c.key(), c.draft.Title, c.draft.Company, c.draft.Location, c.preview().Remote,
				c.draft.SalaryMin, c.draft.SalaryMax, c.draft.Currency, c.draft.EmploymentType,
				textutil.Truncate(c.draft.Description, 1200))
		}
		raw, err := provider.Complete(ctx, system, b.String())
		if err != nil {
			return err
		}
		var items []scoreItem
		if err := json.Unmarshal([]byte(llm.ExtractJSON(raw)), &items); err != nil {
			return fmt.Errorf("score json: %w", err)
		}
		byID := map[string]int{}
		for i, idx := range indexes[start:end] {
			byID[cands[idx].key()] = indexes[start+i]
		}
		threshold := profile.MinScore
		for _, item := range items {
			idx, ok := byID[item.ID]
			if !ok {
				continue
			}
			applyScore(&cands[idx], item, threshold)
		}
	}
	return nil
}

type scoreItem struct {
	ID      string   `json:"id"`
	Score   int      `json:"score"`
	Pros    []string `json:"pros"`
	Cons    []string `json:"cons"`
	Summary string   `json:"summary"`
	Gaps    []string `json:"gaps"`
}

func applyScore(c *candidate, item scoreItem, threshold int) {
	if item.Score < 0 {
		item.Score = 0
	}
	if item.Score > 100 {
		item.Score = 100
	}
	c.score = item.Score
	pros := filter.CleanPoints(item.Pros)
	cons := filter.CleanPoints(item.Cons)
	if len(pros) == 0 && len(cons) == 0 {
		if parsedPros, parsedCons, ok := filter.ParseSummary(strings.TrimSpace(item.Summary + gapsSuffix(item.Gaps))); ok {
			pros, cons = parsedPros, parsedCons
		} else if strings.TrimSpace(item.Summary) != "" {
			pros = filter.CleanPoints([]string{item.Summary})
		}
		if len(cons) == 0 && len(item.Gaps) > 0 {
			cons = filter.CleanPoints(item.Gaps)
		}
	}
	if len(pros) > 0 || len(cons) > 0 {
		c.pros = pros
		c.cons = cons
		c.summary = filter.FormatFit(pros, cons)
	}
	if item.Score >= threshold {
		c.status = model.StatusMatched
		c.reason = "score meets threshold"
	} else {
		c.status = model.StatusRejected
		c.reason = fmt.Sprintf("score %d is below %d", item.Score, threshold)
	}
}

// rankCandidate puts infrastructure and other senior engineering roles ahead of
// loose keyword hits, so the scoring budget is spent on the jobs worth reading.
func rankCandidate(c candidate) int {
	title := strings.ToLower(c.draft.Title)
	n := 0
	for _, word := range []string{"infrastructure", "reliability", "devops", "platform", "kubernetes", "sre", "golang", "distributed"} {
		if strings.Contains(title, word) {
			n += 20
		}
	}
	for _, word := range []string{"engineer", "architect", "developer"} {
		if strings.Contains(title, word) {
			n += 15
		}
	}
	job := c.preview()
	high := job.SalaryMax
	if high == 0 {
		high = job.SalaryMin
	}
	if high >= 200000 {
		n += 25
	}
	if high >= 240000 {
		n += 10
	}
	if job.Remote {
		n += 5
	}
	if strings.Contains(strings.ToLower(job.Location), "seattle") {
		n += 8
	}
	return n
}

func gapsSuffix(gaps []string) string {
	if len(gaps) == 0 {
		return ""
	}
	return " Gaps: " + strings.Join(gaps, "; ")
}

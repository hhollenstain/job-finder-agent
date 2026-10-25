package filter

import (
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

// Explain lists why a posting matched the profile. The model can replace these
// with its own points when it scores the role.
func Explain(p model.Profile, job model.Job) (pros, cons []string) {
	hay := strings.Join([]string{job.Title, job.Company, job.Location, job.Description}, "\n")
	var hits []string
	for _, term := range p.Keywords {
		term = strings.TrimSpace(term)
		if term == "" || !textutil.HasTerm(hay, term) {
			continue
		}
		hits = append(hits, term)
		if len(hits) == 5 {
			break
		}
	}
	if len(hits) > 0 {
		pros = append(pros, "Mentions "+joinWords(hits))
	}
	for _, term := range p.MustHave {
		term = strings.TrimSpace(term)
		if term != "" && textutil.HasTerm(hay, term) {
			pros = append(pros, "Includes "+term)
		}
	}
	if job.Remote {
		pros = append(pros, "Remote")
	} else if p.RemoteOnly || len(cleanTerms(p.Locations)) > 0 {
		cons = append(cons, "Not marked remote")
	}
	locHay := job.Location + "\n" + job.Title
	var locHits []string
	for _, loc := range p.Locations {
		loc = strings.TrimSpace(loc)
		if loc != "" && textutil.HasTerm(locHay, loc) {
			locHits = append(locHits, loc)
		}
	}
	places := cleanTerms(p.Locations)
	if len(locHits) > 0 {
		pros = append(pros, "Location matches "+joinWords(locHits))
	} else if job.Remote && len(places) > 0 {
		cons = append(cons, "Does not mention "+joinWords(places))
	}
	if p.MinSalary > 0 {
		high := job.SalaryMax
		if high == 0 {
			high = job.SalaryMin
		}
		if high <= 0 {
			cons = append(cons, "Pay is not listed")
		} else if high >= p.MinSalary {
			pros = append(pros, "Listed pay meets the minimum")
		}
	}
	if job.EmploymentType != "" && containsFold(p.EmploymentTypes, job.EmploymentType) {
		pros = append(pros, employmentLabel(job.EmploymentType))
	}
	return CleanPoints(pros), CleanPoints(cons)
}

// PointsFor uses stored points, then a scored summary, then the profile rules.
func PointsFor(p model.Profile, job model.Job) (pros, cons []string) {
	if len(job.FitPros) > 0 || len(job.FitCons) > 0 {
		return CleanPoints(job.FitPros), CleanPoints(job.FitCons)
	}
	if pros, cons, ok := ParseSummary(job.FitSummary); ok {
		return pros, cons
	}
	return Explain(p, job)
}

// ParseSummary reads a "+ / -" block or an older summary that ends in "Gaps:".
func ParseSummary(summary string) (pros, cons []string, ok bool) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, nil, false
	}
	lines := strings.Split(summary, "\n")
	var plus, minus []string
	structured := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+ ") || strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "− ") {
			structured = true
			break
		}
	}
	if structured {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "+ "):
				plus = append(plus, strings.TrimSpace(line[2:]))
			case strings.HasPrefix(line, "- "):
				minus = append(minus, strings.TrimSpace(line[2:]))
			case strings.HasPrefix(line, "− "):
				minus = append(minus, strings.TrimSpace(strings.TrimPrefix(line, "− ")))
			}
		}
		return CleanPoints(plus), CleanPoints(minus), len(plus)+len(minus) > 0
	}
	idx := strings.Index(strings.ToLower(summary), "gaps:")
	if idx < 0 {
		return nil, nil, false
	}
	head := strings.TrimSpace(summary[:idx])
	tail := strings.TrimSpace(summary[idx+len("gaps:"):])
	for _, sentence := range splitSentences(head) {
		plus = append(plus, sentence)
	}
	for _, gap := range strings.Split(tail, ";") {
		gap = strings.Trim(strings.TrimSpace(gap), ".")
		if gap != "" {
			minus = append(minus, gap)
		}
	}
	return CleanPoints(plus), CleanPoints(minus), true
}

// FormatFit writes the points as a searchable block.
func FormatFit(pros, cons []string) string {
	var b strings.Builder
	for _, item := range pros {
		b.WriteString("+ ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
	for _, item := range cons {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// CleanPoints trims, drops duplicates, and keeps the first six clauses.
func CleanPoints(items []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.TrimLeft(item, "+-•–—− ")
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		item = textutil.Truncate(item, 180)
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) == 6 {
			break
		}
	}
	return out
}

func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	var b strings.Builder
	for _, r := range text {
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			sentence := strings.TrimSpace(b.String())
			if sentence != "" {
				out = append(out, sentence)
			}
			b.Reset()
		}
	}
	if tail := strings.TrimSpace(b.String()); tail != "" {
		out = append(out, tail)
	}
	return out
}

func joinWords(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

func cleanTerms(items []string) []string {
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func containsFold(items []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == want {
			return true
		}
	}
	return false
}

func employmentLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "full_time", "full-time", "fulltime":
		return "Full-time"
	case "part_time", "part-time", "parttime":
		return "Part-time"
	case "contract":
		return "Contract"
	default:
		return strings.TrimSpace(kind)
	}
}

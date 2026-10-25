package source

import (
	"context"
	"html"
	"net/url"
	"strings"
	"sync"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

const linkedInCap = 36

// searchLinkedIn reads LinkedIn's public guest job search. No account or token.
// It does not log in and does not request anything behind an auth wall.
func searchLinkedIn(ctx context.Context, cfg map[string]string, q Query) ([]model.JobDraft, error) {
	queries := linkedInQueries(cfg, q)
	places := linkedInPlaces(cfg, q)
	want := q.Limit
	if want <= 0 || want > linkedInCap {
		want = linkedInCap
	}
	seen := map[string]bool{}
	var cards []model.JobDraft
	var firstErr error
	for _, place := range places {
		for _, keywords := range queries {
			if len(cards) >= want {
				break
			}
			page, err := fetchLinkedInPage(ctx, keywords, place.name, place.remote, 0)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			for _, card := range page {
				if card.ExternalID == "" || seen[card.ExternalID] {
					continue
				}
				seen[card.ExternalID] = true
				cards = append(cards, card)
				if len(cards) >= want {
					break
				}
			}
		}
	}
	if len(cards) == 0 {
		return nil, firstErr
	}
	enrichLinkedIn(ctx, cards)
	return cards, nil
}

type linkedInPlace struct {
	name   string
	remote bool
}

// linkedInQueries turns the profile into a few short searches.
// One string of every keyword is how the guest search misses infrastructure roles.
func linkedInQueries(cfg map[string]string, q Query) []string {
	if v := firstText(cfg, "query", "what", "keyword"); v != "" {
		return []string{v}
	}
	has := map[string]bool{}
	for _, term := range q.Profile.Keywords {
		has[strings.ToLower(strings.TrimSpace(term))] = true
	}
	notes := strings.ToLower(q.Profile.Notes)
	var out []string
	add := func(query string) {
		if len(out) >= 4 {
			return
		}
		for _, existing := range out {
			if existing == query {
				return
			}
		}
		out = append(out, query)
	}
	if has["infrastructure"] || strings.Contains(notes, "infrastructure") {
		add("infrastructure engineer")
	}
	if has["sre"] || has["site reliability"] || strings.Contains(notes, "sre") {
		add("site reliability engineer")
	}
	if has["devops"] || strings.Contains(notes, "devops") {
		add("devops engineer")
	}
	if has["platform"] || strings.Contains(notes, "platform") {
		add("platform engineer")
	}
	if len(out) == 0 && (has["kubernetes"] || has["golang"] || has["go"]) {
		add("kubernetes golang")
	}
	if len(out) == 0 {
		if len(q.Profile.Keywords) > 0 {
			n := len(q.Profile.Keywords)
			if n > 3 {
				n = 3
			}
			add(strings.Join(q.Profile.Keywords[:n], " "))
		} else {
			add("software engineer")
		}
	}
	return out
}

func linkedInPlaces(cfg map[string]string, q Query) []linkedInPlace {
	if v := strings.TrimSpace(cfg["location"]); v != "" {
		return []linkedInPlace{{name: v, remote: q.Profile.RemoteOnly}}
	}
	var places []linkedInPlace
	for _, loc := range q.Profile.Locations {
		loc = strings.TrimSpace(loc)
		if loc == "" || textutil.LooksRemote(loc) {
			continue
		}
		places = append(places, linkedInPlace{name: loc})
		break
	}
	if q.Profile.IncludeRemote || q.Profile.RemoteOnly {
		places = append(places, linkedInPlace{name: "United States", remote: true})
	}
	if len(places) == 0 {
		places = append(places, linkedInPlace{remote: q.Profile.RemoteOnly})
	}
	return places
}

func firstText(cfg map[string]string, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(cfg[key]); v != "" {
			return v
		}
	}
	return ""
}

func fetchLinkedInPage(ctx context.Context, keywords, location string, remoteOnly bool, start int) ([]model.JobDraft, error) {
	u, err := url.Parse("https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search")
	if err != nil {
		return nil, err
	}
	values := u.Query()
	values.Set("keywords", keywords)
	if location != "" {
		values.Set("location", location)
	}
	if remoteOnly {
		values.Set("f_WT", "2")
	}
	values.Set("start", itoa(start))
	u.RawQuery = values.Encode()
	body, err := getBytes(ctx, u.String(), map[string]string{"Accept": "text/html"})
	if err != nil {
		return nil, err
	}
	return parseLinkedInCards(string(body)), nil
}

func enrichLinkedIn(ctx context.Context, cards []model.JobDraft) {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range cards {
		if cards[i].ExternalID == "" {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			body, err := getBytes(ctx, "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"+url.PathEscape(cards[i].ExternalID), map[string]string{"Accept": "text/html"})
			if err != nil {
				return
			}
			desc := parseLinkedInDescription(string(body))
			if desc == "" {
				return
			}
			cards[i].Description = desc
		}(i)
	}
	wg.Wait()
}

func parseLinkedInCards(page string) []model.JobDraft {
	const marker = `data-entity-urn="urn:li:jobPosting:`
	parts := strings.Split(page, marker)
	if len(parts) < 2 {
		return nil
	}
	out := make([]model.JobDraft, 0, len(parts)-1)
	for _, part := range parts[1:] {
		id, rest, ok := strings.Cut(part, `"`)
		if !ok || strings.TrimSpace(id) == "" {
			continue
		}
		title := cleanLinkedIn(sectionText(rest, "base-search-card__title", "</h3>"))
		company := cleanLinkedIn(sectionText(rest, "base-search-card__subtitle", "</h4>"))
		location := cleanLinkedIn(sectionText(rest, "job-search-card__location", "</span>"))
		link := linkedInJobURL(html.UnescapeString(attrNear(rest, "base-card__full-link", "href")))
		if title == "" || link == "" {
			continue
		}
		if company == "" {
			company = "Unknown"
		}
		remote := textutil.LooksRemote(location + " " + title)
		draft := model.JobDraft{
			ExternalID:  strings.TrimSpace(id),
			Title:       title,
			Company:     company,
			Location:    location,
			URL:         link,
			Description: strings.TrimSpace(strings.Join([]string{title, company, location}, "\n")),
		}
		if remote {
			draft.Remote = &remote
		}
		out = append(out, draft)
	}
	return out
}

func parseLinkedInDescription(page string) string {
	body := innerDiv(page, "show-more-less-html__markup")
	text := textutil.StripHTML(body)
	return textutil.Truncate(strings.TrimSpace(text), 20000)
}

func sectionText(chunk, class, end string) string {
	i := strings.Index(chunk, class)
	if i < 0 {
		return ""
	}
	rest := chunk[i:]
	gt := strings.Index(rest, ">")
	if gt < 0 {
		return ""
	}
	rest = rest[gt+1:]
	stop := strings.Index(strings.ToLower(rest), end)
	if stop < 0 {
		if len(rest) > 400 {
			rest = rest[:400]
		}
		return rest
	}
	return rest[:stop]
}

func attrNear(chunk, marker, attr string) string {
	i := strings.Index(chunk, marker)
	if i < 0 {
		return ""
	}
	window := chunk[i:]
	if len(window) > 800 {
		window = window[:800]
	}
	key := attr + `="`
	j := strings.Index(window, key)
	if j < 0 {
		return ""
	}
	window = window[j+len(key):]
	end := strings.Index(window, `"`)
	if end < 0 {
		return ""
	}
	return window[:end]
}

func innerDiv(page, class string) string {
	idx := strings.Index(page, class)
	if idx < 0 {
		return ""
	}
	open := strings.LastIndex(page[:idx], "<div")
	if open < 0 {
		return ""
	}
	lower := strings.ToLower(page)
	depth := 0
	for i := open; i < len(page); {
		if strings.HasPrefix(lower[i:], "<div") {
			depth++
			i += 4
			continue
		}
		if strings.HasPrefix(lower[i:], "</div") {
			depth--
			end := strings.Index(page[i:], ">")
			if end < 0 {
				return ""
			}
			if depth == 0 {
				content := strings.Index(page[open:], ">")
				if content < 0 {
					return ""
				}
				return page[open+content+1 : i]
			}
			i += end + 1
			continue
		}
		i++
	}
	return ""
}

func cleanLinkedIn(s string) string {
	s = textutil.StripHTML(html.UnescapeString(s))
	return strings.Join(strings.Fields(s), " ")
}

func linkedInJobURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(textutil.CanonicalURL(raw))
	if err != nil || parsed.Host == "" {
		return textutil.CanonicalURL(raw)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

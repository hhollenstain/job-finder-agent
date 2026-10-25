package source

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

// Kinds is the catalog the UI and the agent use when adding a source.
func Kinds() []model.KindInfo {
	return []model.KindInfo{
		{Kind: "remotive", Label: "Remotive", Public: true, Summary: "Remote roles from Remotive's public feed. The app links back to the Remotive posting.", Fields: []model.Field{
			{Name: "query", Label: "Search", Kind: "text", Placeholder: "golang", Help: "Optional. Falls back to your profile keywords."},
		}},
		{Kind: "remoteok", Label: "Remote OK", Public: true, Summary: "Remote roles from Remote OK. Listings link back to Remote OK, which is required by their API terms.", Fields: []model.Field{
			{Name: "query", Label: "Search", Kind: "text", Placeholder: "backend"},
		}},
		{Kind: "arbeitnow", Label: "Arbeitnow", Public: true, Summary: "Public job board feed, strongest in Europe, including visa-sponsorship flags.", Fields: []model.Field{
			{Name: "query", Label: "Search", Kind: "text"},
		}},
		{Kind: "adzuna", Label: "Adzuna", Summary: "Aggregator with a free developer key. Covers many countries. Display attribution when you use the data.", Fields: []model.Field{
			{Name: "app_id", Label: "App ID", Kind: "text", Required: true},
			{Name: "app_key", Label: "App key", Kind: "password", Required: true},
			{Name: "country", Label: "Country code", Kind: "text", Placeholder: "us"},
			{Name: "what", Label: "What", Kind: "text", Placeholder: "golang developer"},
			{Name: "where", Label: "Where", Kind: "text", Placeholder: "remote"},
		}},
		{Kind: "usajobs", Label: "USAJOBS", Summary: "US federal openings. Register a key and use the email on that registration as the user agent.", Fields: []model.Field{
			{Name: "api_key", Label: "API key", Kind: "password", Required: true},
			{Name: "email", Label: "Registration email", Kind: "text", Required: true},
			{Name: "keyword", Label: "Keyword", Kind: "text"},
			{Name: "location", Label: "Location", Kind: "text"},
		}},
		{Kind: "linkedin", Label: "LinkedIn", Public: true, Summary: "Public LinkedIn job search. No account or token. The app reads the guest job listings LinkedIn shows without signing in, and links back to each posting.", Fields: []model.Field{
			{Name: "query", Label: "Search", Kind: "text", Placeholder: "golang", Help: "Optional. Falls back to your profile keywords."},
			{Name: "location", Label: "Location", Kind: "text", Placeholder: "United States", Help: "Optional. Falls back to the first location on your profile."},
		}},
		{Kind: "jsearch", Label: "Indeed and Dice via JSearch", Summary: "Optional RapidAPI JSearch key for publishers that do not offer a public search, including Indeed and Dice. This app does not log in to those sites or bypass their access controls.", Fields: []model.Field{
			{Name: "api_key", Label: "RapidAPI key", Kind: "password", Required: true},
			{Name: "query", Label: "Query", Kind: "text", Placeholder: "senior golang engineer remote"},
			{Name: "publishers", Label: "Publishers", Kind: "text", Placeholder: "LinkedIn, Indeed, Dice", Help: "Optional comma list. Empty keeps every publisher JSearch returns."},
		}},
		{Kind: "greenhouse", Label: "Greenhouse board", Public: true, Summary: "One company's public Greenhouse board, for example stripe.", Fields: []model.Field{
			{Name: "board", Label: "Board token", Kind: "text", Required: true, Placeholder: "stripe"},
			{Name: "company", Label: "Company name", Kind: "text"},
		}},
		{Kind: "lever", Label: "Lever board", Public: true, Summary: "One company's public Lever postings site.", Fields: []model.Field{
			{Name: "site", Label: "Site name", Kind: "text", Required: true, Placeholder: "netflix"},
			{Name: "company", Label: "Company name", Kind: "text"},
		}},
		{Kind: "ashby", Label: "Ashby board", Public: true, Summary: "One company's public Ashby job board.", Fields: []model.Field{
			{Name: "board", Label: "Board name", Kind: "text", Required: true, Placeholder: "notion"},
			{Name: "company", Label: "Company name", Kind: "text"},
		}},
		{Kind: "rss", Label: "RSS or Atom feed", Public: true, Summary: "Any feed you are allowed to read, including a board that still publishes RSS.", Fields: []model.Field{
			{Name: "url", Label: "Feed URL", Kind: "text", Required: true, Placeholder: "https://example.com/jobs.rss"},
			{Name: "company", Label: "Company name", Kind: "text", Help: "Used when the feed does not name one."},
		}},
		{Kind: "slack", Label: "Slack channel", Summary: "Reads job posts from channels your bot is already in. Needs channels:history (and groups:history for private channels).", Fields: []model.Field{
			{Name: "token", Label: "Bot token", Kind: "password", Required: true, Placeholder: "xoxb-..."},
			{Name: "channels", Label: "Channel IDs", Kind: "text", Required: true, Placeholder: "C012ABCDEF,C045GHIJKL"},
			{Name: "query", Label: "Message contains", Kind: "text", Help: "Optional. Falls back to profile keywords."},
		}},
		{Kind: "json", Label: "JSON feed", Public: true, Summary: "A JSON document you can describe with field paths. The agent can register one of these when you point it at a feed.", Fields: []model.Field{
			{Name: "url", Label: "URL", Kind: "text", Required: true},
			{Name: "items_path", Label: "Items path", Kind: "text", Placeholder: "jobs", Help: "Dot path to the array. Leave empty to try jobs, data, or results."},
			{Name: "title", Label: "Title path", Kind: "text", Placeholder: "title"},
			{Name: "company", Label: "Company path", Kind: "text", Placeholder: "company"},
			{Name: "link", Label: "URL path", Kind: "text", Placeholder: "url"},
			{Name: "description", Label: "Description path", Kind: "text", Placeholder: "description"},
			{Name: "location", Label: "Location path", Kind: "text", Placeholder: "location"},
			{Name: "authorization", Label: "Bearer token", Kind: "password"},
		}},
	}
}

// Known reports whether kind is a built-in adapter.
func Known(kind string) bool {
	for _, k := range Kinds() {
		if k.Kind == kind {
			return true
		}
	}
	return false
}

var slugRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,80}$`)

// Validate checks the config for a kind before it is saved.
func Validate(kind string, cfg map[string]string) error {
	if !Known(kind) {
		return fmt.Errorf("unknown source kind %q", kind)
	}
	need := func(key, label string) error {
		if strings.TrimSpace(cfg[key]) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
	switch kind {
	case "adzuna":
		if err := need("app_id", "app id"); err != nil {
			return err
		}
		return need("app_key", "app key")
	case "usajobs":
		if err := need("api_key", "api key"); err != nil {
			return err
		}
		return need("email", "registration email")
	case "jsearch":
		return need("api_key", "RapidAPI key")
	case "greenhouse":
		if err := need("board", "board token"); err != nil {
			return err
		}
		if !slugRE.MatchString(strings.TrimSpace(cfg["board"])) {
			return fmt.Errorf("board token must be a slug such as stripe")
		}
	case "lever":
		if err := need("site", "site name"); err != nil {
			return err
		}
		if !slugRE.MatchString(strings.TrimSpace(cfg["site"])) {
			return fmt.Errorf("site name must be a slug such as netflix")
		}
	case "ashby":
		if err := need("board", "board name"); err != nil {
			return err
		}
		if !slugRE.MatchString(strings.TrimSpace(cfg["board"])) {
			return fmt.Errorf("board name must be a slug such as notion")
		}
	case "rss", "json":
		raw := strings.TrimSpace(cfg["url"])
		if raw == "" {
			return fmt.Errorf("url is required")
		}
		if err := CheckURL(raw); err != nil {
			return err
		}
	case "slack":
		if err := need("token", "bot token"); err != nil {
			return err
		}
		if err := need("channels", "channel ids"); err != nil {
			return err
		}
		for _, ch := range splitComma(cfg["channels"]) {
			if len(ch) < 2 {
				return fmt.Errorf("channel id %q is too short", ch)
			}
		}
	}
	return nil
}

// SameIdentity reports whether two sources point at the same feed or board.
func SameIdentity(a, b model.Source) bool {
	if a.Kind != b.Kind {
		return false
	}
	keys := []string{"url", "board", "site", "channels", "publishers", "app_id"}
	for _, key := range keys {
		av := strings.ToLower(strings.TrimSpace(a.Config[key]))
		bv := strings.ToLower(strings.TrimSpace(b.Config[key]))
		if av != "" || bv != "" {
			return av == bv && av != ""
		}
	}
	return false
}

func cfgOr(cfg map[string]string, key, fallback string) string {
	if v := strings.TrimSpace(cfg[key]); v != "" {
		return v
	}
	return fallback
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func queryEscape(s string) string { return url.QueryEscape(s) }

func companyOr(cfg map[string]string, fallback string) string {
	if c := strings.TrimSpace(cfg["company"]); c != "" {
		return c
	}
	if fallback == "" {
		return "Unknown"
	}
	return fallback
}

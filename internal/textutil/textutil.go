// Package textutil holds small string helpers used while normalizing postings.
package textutil

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var tagRE = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>|<[^>]+>`)
var spaceRE = regexp.MustCompile(`[ \t]+`)
var nlRE = regexp.MustCompile(`\n{3,}`)

// StripHTML removes tags and collapses whitespace so descriptions can be stored as text.
func StripHTML(s string) string {
	s = tagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(spaceRE.ReplaceAllString(line, " "))
	}
	s = strings.TrimSpace(strings.Join(lines, "\n"))
	return strings.TrimSpace(nlRE.ReplaceAllString(s, "\n\n"))
}

// Truncate cuts s to at most n bytes on a rune boundary.
func Truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut])
}

func utf8Start(b byte) bool {
	return b&0xC0 != 0x80
}

// Slug makes a filesystem-safe fragment.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	return out
}

// Hash returns a short stable id.
func Hash(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(h[:])[:16]
}

// CanonicalURL strips tracking parameters and normalizes the host for dedupe.
func CanonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(raw)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	q := u.Query()
	for key := range q {
		lk := strings.ToLower(key)
		if strings.HasPrefix(lk, "utm_") || lk == "gh_src" || lk == "lever-source" || lk == "ref" || lk == "source" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = ""
	if u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String()
}

// Fingerprint identifies the same role across sources.
func Fingerprint(company, title, location, rawURL string) string {
	if c := CanonicalURL(rawURL); c != "" {
		return Hash("url", c)
	}
	return Hash("text", strings.ToLower(company), strings.ToLower(title), strings.ToLower(location))
}

// LooksRemote reports whether a location string describes remote work.
func LooksRemote(location string) bool {
	l := strings.ToLower(strings.TrimSpace(location))
	if l == "" {
		return false
	}
	for _, term := range []string{"remote", "anywhere", "distributed", "work from home", "wfh"} {
		if strings.Contains(l, term) {
			return true
		}
	}
	return false
}

// HasTerm is a case-insensitive match. Terms shorter than two characters never match.
// Terms of two or three characters must be a whole word, so "go" does not hit "government".
func HasTerm(haystack, term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if len(term) < 2 {
		return false
	}
	hay := strings.ToLower(haystack)
	if len(term) >= 4 {
		return strings.Contains(hay, term)
	}
	for i := 0; i+len(term) <= len(hay); i++ {
		if hay[i:i+len(term)] != term {
			continue
		}
		before := i == 0 || !isWordByte(hay[i-1])
		after := i+len(term) == len(hay) || !isWordByte(hay[i+len(term)])
		if before && after {
			return true
		}
	}
	return false
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// CleanList trims, drops blanks, and caps the list.
func CleanList(in []string, max int) []string {
	if max <= 0 {
		max = 50
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) >= max {
			break
		}
	}
	return out
}

// ParseTime accepts the date shapes job boards commonly return.
func ParseTime(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" || v == "null" {
		return nil
	}
	if isDigits(v) {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil
		}
		if n > 1_000_000_000_000 {
			n = n / 1000
		}
		if n < 1_000_000_000 {
			return nil
		}
		t := time.Unix(n, 0).UTC()
		return &t
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"Mon, 02 Jan 2006 15:04:05 MST",
		"Mon, 02 Jan 2006 15:04:05 -0700",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SafePathPrefix keeps Kiwi and local context writes inside a relative folder.
func SafePathPrefix(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "/")
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" {
		return "job-search", nil
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", errBadPrefix
		}
	}
	return strings.Join(parts, "/"), nil
}

// SafeRel joins a prefix and a relative file path and rejects escapes.
func SafeRel(prefix, rel string) (string, error) {
	prefix, err := SafePathPrefix(prefix)
	if err != nil {
		return "", err
	}
	rel = strings.TrimSpace(rel)
	rel = strings.Trim(rel, "/")
	rel = strings.ReplaceAll(rel, "\\", "/")
	if rel == "" {
		return "", errBadPrefix
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return "", errBadPrefix
		}
	}
	return prefix + "/" + rel, nil
}

var errBadPrefix = errString("invalid context path")

type errString string

func (e errString) Error() string { return string(e) }

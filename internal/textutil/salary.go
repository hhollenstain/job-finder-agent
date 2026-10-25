package textutil

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var thousandsDot = regexp.MustCompile(`^\d{1,3}(?:\.\d{3})+$`)
var leadingRange = regexp.MustCompile(`(?i)(USD|EUR|GBP|CAD|AUD|US\$|\$|£|€)\s*(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?[kK]?)\s*(?:-|to|and)\s*(?:USD|EUR|GBP|CAD|AUD|US\$|\$|£|€)?\s*(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?[kK]?)`)
var trailingRange = regexp.MustCompile(`(?i)(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?[kK]?)\s*(?:-|to)\s*(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?[kK]?)\s*(USD|EUR|GBP|CAD|AUD|\$|£|€)`)
var singleMoney = regexp.MustCompile(`(?i)(USD|EUR|GBP|CAD|AUD|US\$|\$|£|€)\s*(\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?[kK]?)\b`)

// ParseSalary finds a pay range in posting text. Amounts are yearly or hourly as written.
// A match requires a currency symbol or code so years and headcount are left alone.
// A lone small figure is kept only when it sits next to pay language, so a donation match is ignored.
func ParseSalary(text string) (min, max int, currency string, ok bool) {
	text = strings.NewReplacer("\u00a0", " ", "\u2013", "-", "\u2014", "-", "\u2212", "-").Replace(text)
	if m := leadingRange.FindStringSubmatch(text); m != nil {
		low, high := parseAmount(m[2]), parseAmount(m[3])
		return finishSalary(low, high, currencyCode(m[1]))
	}
	if m := trailingRange.FindStringSubmatch(text); m != nil {
		low, high := parseAmount(m[1]), parseAmount(m[2])
		return finishSalary(low, high, currencyCode(m[3]))
	}
	for _, m := range singleMoney.FindAllStringSubmatchIndex(text, -1) {
		amount := parseAmount(text[m[4]:m[5]])
		if amount < 20000 && !salaryContext(text, m[0]) {
			continue
		}
		if amount < 1000 {
			continue
		}
		return finishSalary(amount, amount, currencyCode(text[m[2]:m[3]]))
	}
	return 0, 0, "", false
}

func salaryContext(text string, at int) bool {
	start := at - 80
	if start < 0 {
		start = 0
	}
	end := at + 48
	if end > len(text) {
		end = len(text)
	}
	window := strings.ToLower(text[start:end])
	for _, word := range []string{
		"salary", "compensation", "hiring range", "per year", "per annum", "annually",
		"a year", "per hour", "hourly", "/hour", "p/hr", "per task",
	} {
		if strings.Contains(window, word) {
			return true
		}
	}
	return false
}

func finishSalary(low, high int, currency string) (int, int, string, bool) {
	if low <= 0 || high <= 0 {
		return 0, 0, "", false
	}
	if low > high {
		low, high = high, low
	}
	if low < 10 {
		return 0, 0, "", false
	}
	return low, high, currency, true
}

func parseAmount(raw string) int {
	raw = strings.TrimSpace(strings.ToLower(raw))
	k := strings.HasSuffix(raw, "k")
	if k {
		raw = strings.TrimSuffix(raw, "k")
	}
	if thousandsDot.MatchString(raw) {
		raw = strings.ReplaceAll(raw, ".", "")
	}
	raw = strings.ReplaceAll(raw, ",", "")
	raw = strings.TrimSpace(raw)
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return 0
	}
	if k {
		value *= 1000
	}
	return int(math.Round(value))
}

func currencyCode(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "$", "US$", "USD":
		return "USD"
	case "£", "GBP":
		return "GBP"
	case "€", "EUR":
		return "EUR"
	case "CAD":
		return "CAD"
	case "AUD":
		return "AUD"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

package textutil

import "testing"

func TestStripHTML(t *testing.T) {
	got := StripHTML("<p>Hello <b>Go</b></p><script>alert(1)</script>")
	if got != "Hello Go" {
		t.Fatalf("got %q", got)
	}
}

func TestCanonicalURL(t *testing.T) {
	got := CanonicalURL("https://Example.com/jobs/1/?utm_source=feed&ref=x")
	if got != "https://example.com/jobs/1" {
		t.Fatalf("got %q", got)
	}
}

func TestFingerprintSameURL(t *testing.T) {
	a := Fingerprint("A", "Eng", "Remote", "https://example.com/j/1?utm_medium=email")
	b := Fingerprint("B", "Other", "NYC", "https://example.com/j/1")
	if a != b {
		t.Fatalf("urls should dedupe: %s vs %s", a, b)
	}
}

func TestParseTimeUnix(t *testing.T) {
	got := ParseTime("1700000000")
	if got == nil || got.Unix() != 1700000000 {
		t.Fatalf("got %v", got)
	}
}

func TestSafePathPrefix(t *testing.T) {
	if _, err := SafePathPrefix("../secret"); err == nil {
		t.Fatal("expected error")
	}
	got, err := SafePathPrefix("/job-search/team/")
	if err != nil || got != "job-search/team" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestHasTermShortWord(t *testing.T) {
	if HasTerm("government cargo", "go") {
		t.Fatal("go should not match inside a longer word")
	}
	if !HasTerm("Senior Go engineer", "go") {
		t.Fatal("expected a whole-word match")
	}
}

func TestSlug(t *testing.T) {
	if got := Slug("Senior Go Engineer!"); got != "senior-go-engineer" {
		t.Fatalf("got %q", got)
	}
}

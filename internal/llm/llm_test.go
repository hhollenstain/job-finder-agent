package llm

import "testing"

func TestExtractJSONArray(t *testing.T) {
	got := ExtractJSON("Here you go:\n```json\n[{\"id\":\"a\",\"score\":80}]\n```")
	if got != `[{"id":"a","score":80}]` {
		t.Fatalf("got %q", got)
	}
}

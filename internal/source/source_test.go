package source

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

func TestParseRSSAndAtom(t *testing.T) {
	rss := []byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Acme</title>
		<item><title>Go Engineer</title><link>https://example.com/jobs/1</link><description>&lt;p&gt;Distributed systems&lt;/p&gt;</description><guid>1</guid><pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate></item>
		</channel></rss>`)
	jobs, err := parseFeed(rss, map[string]string{}, 10)
	if err != nil || len(jobs) != 1 || jobs[0].Company != "Acme" || !strings.Contains(jobs[0].Description, "Distributed") {
		t.Fatalf("rss %#v err %v", jobs, err)
	}
	atom := []byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Board</title>
		<entry><title>Backend</title><id>tag:1</id><updated>2024-01-02T15:04:05Z</updated><link href="https://example.com/a"/><summary>APIs</summary></entry>
		</feed>`)
	jobs, err = parseFeed(atom, map[string]string{"company": "Board Co"}, 10)
	if err != nil || len(jobs) != 1 || jobs[0].URL != "https://example.com/a" || jobs[0].Company != "Board Co" {
		t.Fatalf("atom %#v err %v", jobs, err)
	}
}

func TestJSONFeedPaths(t *testing.T) {
	body := []byte(`{"results":[{"id":"9","name":"Staff Engineer","company_name":"North","url":"https://example.com/9","description":"<b>Go</b>","location":"Remote"}]}`)
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	items := dig(root, "results").([]any)
	if asString(dig(items[0], "company_name")) != "North" {
		t.Fatal(items[0])
	}
	if text := asString(dig(items[0], "description")); !strings.Contains(text, "Go") {
		t.Fatal(text)
	}
}

func TestValidateRejectsMetadata(t *testing.T) {
	err := Validate("json", map[string]string{"url": "http://169.254.169.254/latest"})
	if err == nil {
		t.Fatal("expected metadata rejection")
	}
}

func TestSameIdentity(t *testing.T) {
	a := model.Source{Kind: "greenhouse", Config: map[string]string{"board": "Stripe"}}
	b := model.Source{Kind: "greenhouse", Config: map[string]string{"board": "stripe"}}
	if !SameIdentity(a, b) {
		t.Fatal("expected same board")
	}
}

func TestFlexIntString(t *testing.T) {
	var n FlexInt
	if err := n.UnmarshalJSON([]byte(`"120000.5"`)); err != nil || int(n) != 120000 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

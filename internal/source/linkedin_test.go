package source

import (
	"strings"
	"testing"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

func TestParseLinkedInCards(t *testing.T) {
	page := `
<li>
<div class="base-search-card job-search-card" data-entity-urn="urn:li:jobPosting:4461900818">
  <a class="base-card__full-link" href="https://www.linkedin.com/jobs/view/senior-go-lang-engineer-at-epam-systems-4461900818?trackingId=abc&amp;refId=z">
    <span class="sr-only">Senior Go Lang Engineer</span>
  </a>
  <h3 class="base-search-card__title">Senior Go Lang Engineer</h3>
  <h4 class="base-search-card__subtitle"><a href="https://www.linkedin.com/company/epam-systems">EPAM Systems</a></h4>
  <span class="job-search-card__location">Chicago, IL</span>
</div>
</li>`
	jobs := parseLinkedInCards(page)
	if len(jobs) != 1 {
		t.Fatalf("jobs %#v", jobs)
	}
	job := jobs[0]
	if job.ExternalID != "4461900818" || job.Title != "Senior Go Lang Engineer" || job.Company != "EPAM Systems" || job.Location != "Chicago, IL" {
		t.Fatalf("%#v", job)
	}
	if strings.Contains(job.URL, "trackingId") || !strings.Contains(job.URL, "/jobs/view/") {
		t.Fatalf("url %s", job.URL)
	}
}

func TestParseLinkedInDescription(t *testing.T) {
	page := `<div class="description__text"><div class="show-more-less-html__markup">We are seeking an engineer.<br><br>Salary range: $130,000 - $150,000.</div></div>`
	text := parseLinkedInDescription(page)
	if !strings.Contains(text, "Salary range: $130,000 - $150,000.") {
		t.Fatalf("%q", text)
	}
}

func TestLinkedInQueriesStayShort(t *testing.T) {
	q := Query{Profile: model.Profile{
		Notes:         "Looking for senior infrastructure, platform, DevOps, or SRE roles.",
		Keywords:      []string{"python", "golang", "kubernetes", "devops", "sre", "platform", "distributed", "gcp", "infrastructure"},
		Locations:     []string{"Seattle", "remote"},
		IncludeRemote: true,
	}}
	queries := linkedInQueries(nil, q)
	if len(queries) != 4 || queries[0] != "infrastructure engineer" || queries[1] != "site reliability engineer" {
		t.Fatalf("%#v", queries)
	}
	places := linkedInPlaces(nil, q)
	if len(places) != 2 || places[0].name != "Seattle" || !places[1].remote || places[1].name != "United States" {
		t.Fatalf("%#v", places)
	}
}

func TestLinkedInNeedsNoToken(t *testing.T) {
	if err := Validate("linkedin", map[string]string{}); err != nil {
		t.Fatal(err)
	}
}

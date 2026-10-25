package filter

import (
	"testing"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

func job(title, body, loc string, remote bool, salary int) model.Job {
	return model.Job{
		Title:       title,
		Company:     "Acme",
		Location:    loc,
		Remote:      remote,
		Description: body,
		SalaryMax:   salary,
	}
}

func TestDecideKeywordsAndExclude(t *testing.T) {
	p := model.Profile{
		Keywords:      []string{"golang"},
		Exclude:       []string{"unpaid"},
		IncludeRemote: true,
	}
	if d := Decide(p, job("Intern", "unpaid golang role", "Remote", true, 0)); d.Pass {
		t.Fatal("exclude should win")
	}
	if d := Decide(p, job("Designer", "figma systems", "Remote", true, 0)); d.Pass {
		t.Fatal("keyword should be required")
	}
	if d := Decide(p, job("Engineer", "We write Golang services", "Remote", true, 0)); !d.Pass {
		t.Fatal(d.Reason)
	}
}

func TestDecideRemoteAndSalary(t *testing.T) {
	p := model.Profile{RemoteOnly: true, MinSalary: 100000}
	if d := Decide(p, job("Engineer", "office", "Austin", false, 0)); d.Pass {
		t.Fatal("expected remote rejection")
	}
	if d := Decide(p, job("Engineer", "remote", "Remote", true, 80000)); d.Pass {
		t.Fatal("expected salary rejection")
	}
	if d := Decide(p, job("Engineer", "remote", "Remote", true, 0)); !d.Pass {
		t.Fatal("unknown salary should pass")
	}
}

func TestDecideLocationAllowsRemote(t *testing.T) {
	p := model.Profile{Locations: []string{"Austin"}, IncludeRemote: true}
	if d := Decide(p, job("Engineer", "backend", "Remote - US", true, 0)); !d.Pass {
		t.Fatal(d.Reason)
	}
	p.IncludeRemote = false
	if d := Decide(p, job("Engineer", "backend", "Remote - US", true, 0)); d.Pass {
		t.Fatal("remote should not satisfy Austin when include_remote is off")
	}
}

func TestDecideNationwideCountsAsRemote(t *testing.T) {
	p := model.Profile{Locations: []string{"Seattle"}, IncludeRemote: true, MinSalary: 200000}
	us := job("Staff Site Reliability Engineer", "Kubernetes platform", "United States", false, 326000)
	if d := Decide(p, us); !d.Pass {
		t.Fatal(d.Reason)
	}
	city := job("Software Engineer, Platform", "platform", "New York, United States", false, 312000)
	if d := Decide(p, city); d.Pass {
		t.Fatal("a named city should not match Seattle")
	}
	low := job("Infrastructure Engineer", "kubernetes", "Seattle, WA", false, 180000)
	if d := Decide(p, low); d.Pass {
		t.Fatal(d.Reason)
	}
	p.Locations = []string{"Seattle", "remote"}
	onsite := job("Staff Engineer", "A remote-friendly culture in the office", "San Diego, CA", false, 262000)
	if d := Decide(p, onsite); d.Pass {
		t.Fatal("mentioning remote in the body should not move San Diego to Seattle")
	}
}

// Package filter applies the profile as hard rules before any model call.
package filter

import (
	"fmt"
	"strings"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

// Decision is the deterministic outcome for one posting.
type Decision struct {
	Pass   bool
	Reason string
}

// Decide drops postings that miss a must-have, hit an exclude term, or fail location and pay rules.
// Salary is compared as a raw number. Unknown pay is kept. Currency is not converted.
func Decide(p model.Profile, job model.Job) Decision {
	hay := strings.Join([]string{
		job.Title,
		job.Company,
		job.Location,
		job.Description,
		job.EmploymentType,
	}, "\n")

	for _, term := range p.Exclude {
		if textutil.HasTerm(hay, term) {
			return Decision{Reason: "excluded term: " + term}
		}
	}
	for _, term := range p.MustHave {
		if !textutil.HasTerm(hay, term) {
			return Decision{Reason: "missing required term: " + term}
		}
	}
	if len(p.Keywords) > 0 {
		ok := false
		for _, term := range p.Keywords {
			if textutil.HasTerm(hay, term) {
				ok = true
				break
			}
		}
		if !ok {
			return Decision{Reason: "no keyword match"}
		}
	}
	if p.RemoteOnly && !job.Remote {
		return Decision{Reason: "not remote"}
	}
	if len(p.Locations) > 0 && !p.RemoteOnly {
		locOK := false
		for _, loc := range p.Locations {
			if textutil.LooksRemote(loc) {
				continue
			}
			if textutil.HasTerm(job.Location, loc) || textutil.HasTerm(job.Title+" "+job.Description, loc) {
				locOK = true
				break
			}
		}
		if p.IncludeRemote && (job.Remote || nationwide(job.Location)) {
			locOK = true
		}
		if !locOK {
			return Decision{Reason: "location does not match"}
		}
	}
	if p.MinSalary > 0 && job.SalaryMax > 0 && job.SalaryMax < p.MinSalary {
		return Decision{Reason: fmt.Sprintf("pay ceiling %d is below minimum %d", job.SalaryMax, p.MinSalary)}
	}
	if len(p.EmploymentTypes) > 0 && job.EmploymentType != "" {
		want := map[string]bool{}
		for _, kind := range p.EmploymentTypes {
			want[normalizeType(kind)] = true
		}
		if !want[normalizeType(job.EmploymentType)] {
			return Decision{Reason: "employment type " + job.EmploymentType}
		}
	}
	return Decision{Pass: true, Reason: "passed rules"}
}

// nationwide is a posting that names a country and no city, such as "United States".
func nationwide(location string) bool {
	loc := strings.ToLower(strings.TrimSpace(location))
	loc = strings.Trim(loc, " .,")
	switch loc {
	case "united states", "united states of america", "usa", "us", "u.s", "u.s.a", "north america":
		return true
	default:
		return false
	}
}

func normalizeType(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	switch s {
	case "fulltime", "full_time", "permanent":
		return "full_time"
	case "parttime", "part_time":
		return "part_time"
	case "contract", "contractor", "freelance":
		return "contract"
	case "intern", "internship":
		return "intern"
	default:
		return s
	}
}

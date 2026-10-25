package filter

import (
	"strings"
	"testing"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

func TestExplainMatchReasons(t *testing.T) {
	p := model.Profile{
		Keywords:        []string{"golang", "kubernetes"},
		Locations:       []string{"Seattle"},
		IncludeRemote:   true,
		MinSalary:       240000,
		EmploymentTypes: []string{"full_time"},
	}
	remote := job("Golang platform", "Kubernetes services", "Remote", true, 0)
	remote.EmploymentType = "full_time"
	pros, cons := Explain(p, remote)
	joined := strings.Join(pros, " | ")
	if !strings.Contains(joined, "golang") || !strings.Contains(joined, "Remote") || !strings.Contains(joined, "Full-time") {
		t.Fatalf("pros: %#v", pros)
	}
	if !hasPoint(cons, "Pay is not listed") || !hasPoint(cons, "Does not mention Seattle") {
		t.Fatalf("cons: %#v", cons)
	}

	onsite := job("Golang platform", "Seattle office", "Seattle, WA", false, 300000)
	onsite.SalaryMin = 260000
	pros, cons = Explain(p, onsite)
	if !hasPoint(pros, "Location matches Seattle") || !hasPoint(pros, "Listed pay meets the minimum") {
		t.Fatalf("pros: %#v", pros)
	}
	if !hasPoint(cons, "Not marked remote") {
		t.Fatalf("cons: %#v", cons)
	}
}

func TestParseSummaryGaps(t *testing.T) {
	summary := "Title matches golang. The posting is thin. Gaps: pay unspecified; location is Santa Clara"
	pros, cons, ok := ParseSummary(summary)
	if !ok || len(pros) != 2 || len(cons) != 2 {
		t.Fatalf("ok=%v pros=%#v cons=%#v", ok, pros, cons)
	}
	if cons[0] != "pay unspecified" || cons[1] != "location is Santa Clara" {
		t.Fatalf("%#v", cons)
	}
	text := FormatFit(pros, cons)
	again, againCons, ok := ParseSummary(text)
	if !ok || len(again) != 2 || len(againCons) != 2 {
		t.Fatalf("round trip %#v %#v", again, againCons)
	}
}

func hasPoint(items []string, want string) bool {
	for _, item := range items {
		if item == want || strings.Contains(item, want) {
			return true
		}
	}
	return false
}

package agent

import (
	"strings"
	"testing"

	"github.com/henryhollenstain/job-finder-agent/internal/model"
)

func TestRankPrefersInfrastructurePay(t *testing.T) {
	infra := candidate{draft: model.JobDraft{Title: "Infrastructure Engineer", Location: "Seattle, WA", SalaryMin: 200000, SalaryMax: 260000}}
	design := candidate{draft: model.JobDraft{Title: "Graphic Designer", Location: "Remote"}}
	if rankCandidate(infra) <= rankCandidate(design) {
		t.Fatalf("infra %d design %d", rankCandidate(infra), rankCandidate(design))
	}
}

func TestApplyScorePoints(t *testing.T) {
	c := candidate{pros: []string{"Remote"}, cons: []string{"Pay is not listed"}, summary: "+ Remote\n- Pay is not listed"}
	applyScore(&c, scoreItem{
		Score: 88,
		Pros:  []string{"+ Mentions Go", "Senior platform work"},
		Cons:  []string{"- Pay is not listed", "Santa Clara, not Seattle"},
	}, 70)
	if c.score != 88 || c.status != "matched" || c.reason != "score meets threshold" {
		t.Fatalf("%#v", c)
	}
	if len(c.pros) != 2 || c.pros[0] != "Mentions Go" || len(c.cons) != 2 {
		t.Fatalf("points %#v %#v", c.pros, c.cons)
	}
	if !strings.Contains(c.summary, "+ Mentions Go") || !strings.Contains(c.summary, "- Santa Clara, not Seattle") {
		t.Fatalf("summary %q", c.summary)
	}

	kept := c
	applyScore(&kept, scoreItem{Score: 40}, 70)
	if kept.score != 40 || kept.status != "rejected" {
		t.Fatalf("reject %#v", kept)
	}
	if len(kept.pros) != 2 || kept.pros[0] != "Mentions Go" {
		t.Fatalf("empty score should keep points, got %#v", kept.pros)
	}
}

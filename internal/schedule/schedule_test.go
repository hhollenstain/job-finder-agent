package schedule

import (
	"testing"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/agent"
)

func TestApplyWeekdayMorning(t *testing.T) {
	r := New(&agent.Agent{})
	if err := r.Apply("0 8 * * 1-5", "UTC"); err != nil {
		t.Fatal(err)
	}
	next := r.Next()
	if next == nil {
		t.Fatal("expected next run")
	}
	if next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
		t.Fatalf("next landed on weekend: %s", next)
	}
	if err := r.Apply("", "UTC"); err != nil {
		t.Fatal(err)
	}
	if r.Next() != nil {
		t.Fatal("schedule should be off")
	}
}

func TestRejectBadExpr(t *testing.T) {
	r := New(nil)
	if err := r.Apply("not a cron", "UTC"); err == nil {
		t.Fatal("expected error")
	}
}

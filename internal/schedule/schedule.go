// Package schedule runs searches on a five-field cron expression.
package schedule

import (
	"fmt"
	"sync"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/agent"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/robfig/cron/v3"
)

// Runner keeps a single cron schedule pointed at the agent.
type Runner struct {
	mu     sync.Mutex
	agent  *agent.Agent
	cron   *cron.Cron
	sched  cron.Schedule
	expr   string
	parser cron.Parser
}

// New returns a runner with standard cron fields plus descriptors such as @daily.
func New(ag *agent.Agent) *Runner {
	return &Runner{
		agent: ag,
		parser: cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		),
	}
}

// Apply replaces the active schedule. An empty expression disables it.
// A bad expression leaves the previous schedule running.
func (r *Runner) Apply(expr, timezone string) error {
	expr = trim(expr)
	var (
		loc   *time.Location
		sched cron.Schedule
		err   error
	)
	if expr != "" {
		loc, err = load(timezone)
		if err != nil {
			return err
		}
		sched, err = r.parser.Parse(expr)
		if err != nil {
			return fmt.Errorf("invalid schedule: %w", err)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cron != nil {
		stop := r.cron.Stop()
		<-stop.Done()
		r.cron = nil
		r.sched = nil
		r.expr = ""
	}
	if expr == "" {
		return nil
	}
	c := cron.New(cron.WithLocation(loc), cron.WithParser(r.parser))
	if _, err := c.AddFunc(expr, func() {
		defer func() { _ = recover() }()
		if r.agent == nil {
			return
		}
		_, _ = r.agent.Start(model.TriggerSchedule, false)
	}); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	c.Start()
	r.cron = c
	r.sched = sched
	r.expr = expr
	return nil
}

// Next is the following run time, or nil when scheduling is off.
func (r *Runner) Next() *time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sched == nil {
		return nil
	}
	t := r.sched.Next(time.Now())
	return &t
}

// Expr returns the active expression.
func (r *Runner) Expr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.expr
}

func load(timezone string) (*time.Location, error) {
	timezone = trim(timezone)
	if timezone == "" || timezone == "Local" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q", timezone)
	}
	return loc, nil
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

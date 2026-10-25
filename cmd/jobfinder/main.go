package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/agent"
	"github.com/henryhollenstain/job-finder-agent/internal/config"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/schedule"
	"github.com/henryhollenstain/job-finder-agent/internal/server"
	"github.com/henryhollenstain/job-finder-agent/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "optional YAML used to seed a new database")
	addrFlag := flag.String("addr", "", "listen address (default 127.0.0.1:8787)")
	dataFlag := flag.String("data", "", "data directory (default ./data)")
	searchFlag := flag.Bool("search", false, "run one search at startup; otherwise wait for the Jobs button or the schedule")
	flag.Parse()

	file, loaded, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	addr := first(*addrFlag, os.Getenv("JOBFINDER_ADDR"), file.Addr, "127.0.0.1:8787")
	dataDir := first(*dataFlag, os.Getenv("JOBFINDER_DATA"), file.DataDir, "data")
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(abs)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.Close()
	if err := st.FailRunning("interrupted by restart"); err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := seed(st, file, loaded); err != nil {
		log.Fatalf("seed: %v", err)
	}
	if err := ensurePublicLinkedIn(st); err != nil {
		log.Fatalf("linkedin: %v", err)
	}
	if n, err := st.BackfillSalaries(); err != nil {
		log.Fatalf("compensation: %v", err)
	} else if n > 0 {
		log.Printf("filled compensation on %d roles", n)
	}
	profile, err := st.Profile()
	if err != nil {
		log.Fatalf("profile: %v", err)
	}
	if n, err := st.BackfillFitPoints(profile); err != nil {
		log.Fatalf("match reasons: %v", err)
	} else if n > 0 {
		log.Printf("filled match reasons on %d roles", n)
	}
	if n, err := st.Refilter(profile); err != nil {
		log.Fatalf("refilter: %v", err)
	} else if n > 0 {
		log.Printf("reapplied profile to %d roles", n)
	}

	ag := agent.New(st, abs)
	runner := schedule.New(ag)
	settings, err := st.Settings()
	if err != nil {
		log.Fatal(err)
	}
	if err := runner.Apply(settings.Schedule, settings.Timezone); err != nil {
		log.Printf("schedule not started: %v", err)
	}
	if *searchFlag {
		go func() {
			if _, err := ag.Start(model.TriggerStartup, false); err != nil {
				log.Printf("startup search: %v", err)
			}
		}()
	} else if next := runner.Next(); next != nil {
		log.Printf("no startup search; next scheduled run %s", next.Format(time.RFC3339))
	} else {
		log.Printf("no startup search; use Run search or pass -search")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(st, ag, runner, abs).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("job finder listening on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shut)
}

func seed(st *store.Store, file config.File, loaded bool) error {
	done, err := st.Seeded()
	if err != nil || done {
		return err
	}
	now := time.Now().UTC()
	profile := model.DefaultProfile()
	settings := model.DefaultSettings()
	sources := defaultSources(now)
	if loaded {
		if file.Profile.Notes != "" || len(file.Profile.Keywords) > 0 {
			profile = file.Profile
		}
		if file.Settings.LLM.Provider != "" || file.Settings.Schedule != "" {
			settings = file.Settings
		}
		if len(file.Sources) > 0 {
			sources = file.Sources
		}
	}
	if settings.LLM.Provider == "openai" && settings.LLM.APIKey == "" {
		settings.LLM.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if settings.LLM.Provider == "anthropic" && settings.LLM.APIKey == "" {
		settings.LLM.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if settings.LLM.Provider == "ollama" {
		if host := os.Getenv("OLLAMA_HOST"); host != "" && settings.LLM.BaseURL == model.DefaultSettings().LLM.BaseURL {
			settings.LLM.BaseURL = host
		}
	}
	if err := st.SaveProfile(profile); err != nil {
		return err
	}
	if err := st.SaveSettings(settings); err != nil {
		return err
	}
	for _, src := range sources {
		if src.CreatedAt.IsZero() {
			src.CreatedAt = now
		}
		if src.UpdatedAt.IsZero() {
			src.UpdatedAt = now
		}
		if src.Config == nil {
			src.Config = map[string]string{}
		}
		if err := st.InsertSource(src); err != nil {
			return err
		}
	}
	return st.MarkSeeded()
}

func defaultSources(now time.Time) []model.Source {
	blank := func(id, kind, name string) model.Source {
		return model.Source{
			ID: id, Kind: kind, Name: name, Enabled: true,
			Config: map[string]string{}, CreatedAt: now, UpdatedAt: now,
		}
	}
	return []model.Source{
		blank("remotive", "remotive", "Remotive"),
		blank("remoteok", "remoteok", "Remote OK"),
		blank("arbeitnow", "arbeitnow", "Arbeitnow"),
		blank("linkedin", "linkedin", "LinkedIn"),
	}
}

func ensurePublicLinkedIn(st *store.Store) error {
	sources, err := st.ListSources()
	if err != nil {
		return err
	}
	for _, src := range sources {
		if src.Kind == "linkedin" {
			return nil
		}
	}
	now := time.Now().UTC()
	return st.InsertSource(model.Source{
		ID: "linkedin", Kind: "linkedin", Name: "LinkedIn", Enabled: true,
		Config: map[string]string{}, CreatedAt: now, UpdatedAt: now,
	})
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

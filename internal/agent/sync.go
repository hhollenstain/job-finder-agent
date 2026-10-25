package agent

import (
	"context"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/kiwi"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"
)

// Sync writes the profile, the latest run, and matched or saved roles into the context folder.
// When KiwiFS is enabled, the same pages are mirrored there under the configured prefix.
func (a *Agent) Sync(ctx context.Context) (model.SyncStatus, error) {
	profile, err := a.store.Profile()
	if err != nil {
		return model.SyncStatus{}, err
	}
	settings, err := a.store.Settings()
	if err != nil {
		return model.SyncStatus{}, err
	}
	settings = normalizeSettings(settings)
	jobs, err := a.store.JobsForContext(80)
	if err != nil {
		return model.SyncStatus{}, err
	}
	var runPtr *model.Run
	if run, ok, err := a.store.LatestRun(); err != nil {
		return model.SyncStatus{}, err
	} else if ok {
		runPtr = &run
	}
	writer := kiwi.NewWriter(a.dataDir, settings.Kiwi)
	res, err := writer.Write(ctx, kiwi.Pages(profile, runPtr, jobs))
	status := model.SyncStatus{
		At:       time.Now().UTC(),
		LocalDir: res.LocalDir,
		Files:    res.Files,
		Kiwi:     res.Kiwi,
	}
	if status.Kiwi == "" {
		status.Kiwi = "disabled"
	}
	_ = a.store.SaveSync(status)
	return status, err
}

func (a *Agent) kiwiContext(ctx context.Context, settings model.Settings) (string, error) {
	writer := kiwi.NewWriter(a.dataDir, settings.Kiwi)
	pack, err := writer.LoadContext(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	add := func(title, body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		b.WriteString(title)
		b.WriteString(":\n")
		b.WriteString(textutil.Truncate(body, 1500))
		b.WriteString("\n\n")
	}
	add("Playbook", pack.Playbook)
	add("Rules", pack.Rules)
	add("Schema", pack.Schema)
	return b.String(), nil
}

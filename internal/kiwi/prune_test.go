package kiwi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneJobPagesKeepsSavedFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "job-search", "jobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	keepPath := filepath.Join(dir, "co-role-job_saved.md")
	dropPath := filepath.Join(dir, "co-other-job_drop.md")
	notePath := filepath.Join(dir, "notes.md")
	for _, path := range []string{keepPath, dropPath, notePath} {
		if err := os.WriteFile(path, []byte("# page\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, err := PruneJobPages(root, "job-search", map[string]bool{"job_saved": true})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("removed %d", n)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(notePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dropPath); !os.IsNotExist(err) {
		t.Fatalf("drop page still present: %v", err)
	}
}

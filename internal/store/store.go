// Package store persists profiles, sources, jobs, and runs in a local SQLite file.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/henryhollenstain/job-finder-agent/internal/filter"
	"github.com/henryhollenstain/job-finder-agent/internal/model"
	"github.com/henryhollenstain/job-finder-agent/internal/textutil"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS kv (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sources (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL,
  config TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  external_id TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  title TEXT NOT NULL,
  company TEXT NOT NULL,
  location TEXT NOT NULL DEFAULT '',
  remote INTEGER NOT NULL DEFAULT 0,
  url TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  salary_min INTEGER NOT NULL DEFAULT 0,
  salary_max INTEGER NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT '',
  employment_type TEXT NOT NULL DEFAULT '',
  posted_at TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  favorite INTEGER NOT NULL DEFAULT 0,
  score INTEGER NOT NULL DEFAULT -1,
  filter_reason TEXT NOT NULL DEFAULT '',
  fit_summary TEXT NOT NULL DEFAULT '',
  fit_pros TEXT NOT NULL DEFAULT '',
  fit_cons TEXT NOT NULL DEFAULT '',
  research TEXT NOT NULL DEFAULT '',
  first_seen TEXT NOT NULL,
  last_seen TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(source_id, external_id)
);
CREATE INDEX IF NOT EXISTS jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS jobs_fingerprint ON jobs(fingerprint);
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  trigger TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL DEFAULT '',
  fetched INTEGER NOT NULL DEFAULT 0,
  new_count INTEGER NOT NULL DEFAULT 0,
  matched INTEGER NOT NULL DEFAULT 0,
  rejected INTEGER NOT NULL DEFAULT 0,
  message TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS source_cache (
  source_id TEXT PRIMARY KEY,
  fetched_at TEXT NOT NULL,
  jobs TEXT NOT NULL
);
`

// Store is safe for concurrent use via database/sql.
type Store struct {
	db *sql.DB
}

// Open creates the data directory and the SQLite file when they are missing.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "jobfinder.db")
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureJobColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func ensureJobColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(jobs)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !have["fit_pros"] {
		if _, err := db.Exec(`ALTER TABLE jobs ADD COLUMN fit_pros TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !have["fit_cons"] {
		if _, err := db.Exec(`ALTER TABLE jobs ADD COLUMN fit_cons TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !have["favorite"] {
		if _, err := db.Exec(`ALTER TABLE jobs ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Seeded reports whether first-run defaults were already written.
func (s *Store) Seeded() (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM kv WHERE key = 'seeded'`).Scan(&n)
	return n > 0, err
}

// MarkSeeded records that defaults should not be applied again.
func (s *Store) MarkSeeded() error {
	_, err := s.db.Exec(`INSERT INTO kv(key, value) VALUES('seeded', '1') ON CONFLICT(key) DO UPDATE SET value = '1'`)
	return err
}

func (s *Store) putJSON(key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO kv(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(b))
	return err
}

func (s *Store) getJSON(key string, dest any) error {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), dest)
}

// Profile loads the search contract. Missing data returns the default.
func (s *Store) Profile() (model.Profile, error) {
	var p model.Profile
	err := s.getJSON("profile", &p)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DefaultProfile(), nil
	}
	return p, err
}

// SaveProfile replaces the search contract.
func (s *Store) SaveProfile(p model.Profile) error { return s.putJSON("profile", p) }

// Settings loads provider, schedule, and Kiwi options.
func (s *Store) Settings() (model.Settings, error) {
	var cfg model.Settings
	err := s.getJSON("settings", &cfg)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DefaultSettings(), nil
	}
	return cfg, err
}

// SaveSettings replaces settings.
func (s *Store) SaveSettings(cfg model.Settings) error { return s.putJSON("settings", cfg) }

// SaveSync stores the last context write.
func (s *Store) SaveSync(st model.SyncStatus) error { return s.putJSON("last_sync", st) }

// LastSync returns the last context write, or a zero status.
func (s *Store) LastSync() (model.SyncStatus, error) {
	var st model.SyncStatus
	err := s.getJSON("last_sync", &st)
	if errors.Is(err, sql.ErrNoRows) {
		return model.SyncStatus{}, nil
	}
	return st, err
}

// InsertSource adds a source.
func (s *Store) InsertSource(src model.Source) error {
	cfg, err := json.Marshal(src.Config)
	if err != nil {
		return err
	}
	if src.Config == nil {
		cfg = []byte(`{}`)
	}
	_, err = s.db.Exec(`INSERT INTO sources(id, kind, name, enabled, config, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
		src.ID, src.Kind, src.Name, boolInt(src.Enabled), string(cfg), stamp(src.CreatedAt), stamp(src.UpdatedAt))
	return err
}

// UpdateSource replaces name, enabled, and config.
func (s *Store) UpdateSource(src model.Source) error {
	cfg, err := json.Marshal(src.Config)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE sources SET name=?, enabled=?, config=?, updated_at=? WHERE id=?`,
		src.Name, boolInt(src.Enabled), string(cfg), stamp(src.UpdatedAt), src.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteSource removes a source configuration. Stored jobs are kept.
func (s *Store) DeleteSource(id string) error {
	res, err := s.db.Exec(`DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	_, _ = s.db.Exec(`DELETE FROM source_cache WHERE source_id = ?`, id)
	return nil
}

// GetSource loads one source.
func (s *Store) GetSource(id string) (model.Source, error) {
	row := s.db.QueryRow(`SELECT id, kind, name, enabled, config, created_at, updated_at FROM sources WHERE id = ?`, id)
	return scanSource(row)
}

// ListSources returns sources ordered by name.
func (s *Store) ListSources() ([]model.Source, error) {
	rows, err := s.db.Query(`SELECT id, kind, name, enabled, config, created_at, updated_at FROM sources ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSource(sc scanner) (model.Source, error) {
	var src model.Source
	var enabled int
	var cfg, created, updated string
	if err := sc.Scan(&src.ID, &src.Kind, &src.Name, &enabled, &cfg, &created, &updated); err != nil {
		return src, err
	}
	src.Enabled = enabled == 1
	src.Config = map[string]string{}
	if cfg != "" {
		if err := json.Unmarshal([]byte(cfg), &src.Config); err != nil {
			return src, err
		}
	}
	src.CreatedAt = parseStamp(created)
	src.UpdatedAt = parseStamp(updated)
	return src, nil
}

// CacheGet returns a fresh cached fetch when it is newer than ttl.
func (s *Store) CacheGet(sourceID string, ttl time.Duration) ([]model.JobDraft, bool, error) {
	var fetched, raw string
	err := s.db.QueryRow(`SELECT fetched_at, jobs FROM source_cache WHERE source_id = ?`, sourceID).Scan(&fetched, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	at := parseStamp(fetched)
	if ttl > 0 && time.Since(at) > ttl {
		return nil, false, nil
	}
	var jobs []model.JobDraft
	if err := json.Unmarshal([]byte(raw), &jobs); err != nil {
		return nil, false, err
	}
	return jobs, true, nil
}

// CachePut stores the latest fetch for a source.
func (s *Store) CachePut(sourceID string, jobs []model.JobDraft) error {
	b, err := json.Marshal(jobs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO source_cache(source_id, fetched_at, jobs) VALUES(?,?,?)
		ON CONFLICT(source_id) DO UPDATE SET fetched_at=excluded.fetched_at, jobs=excluded.jobs`,
		sourceID, stamp(time.Now().UTC()), string(b))
	return err
}

// UpsertResult describes what happened to one posting.
type UpsertResult struct {
	ID          string
	Created     bool
	DuplicateOf string
}

// UpsertJob inserts a posting or refreshes it. Saved and dismissed rows keep their status.
// A matching fingerprint from another source is treated as the same role.
func (s *Store) UpsertJob(j model.Job) (UpsertResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return UpsertResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var existingFP, existingID string
	err = tx.QueryRow(`SELECT id FROM jobs WHERE fingerprint = ? AND source_id != ? LIMIT 1`,
		j.Fingerprint, j.SourceID).Scan(&existingFP)
	if err == nil {
		existingID = existingFP
		_, _ = tx.Exec(`UPDATE jobs SET last_seen = ? WHERE id = ?`, stamp(j.LastSeen), existingID)
		if err := tx.Commit(); err != nil {
			return UpsertResult{}, err
		}
		committed = true
		return UpsertResult{ID: existingID, DuplicateOf: existingID}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return UpsertResult{}, err
	}

	var id, status, research, first, currency, fitSummary, fitPros, fitCons string
	var score, salaryMin, salaryMax, favorite int
	err = tx.QueryRow(`SELECT id, status, favorite, score, research, first_seen, salary_min, salary_max, currency, fit_summary, fit_pros, fit_cons FROM jobs WHERE source_id = ? AND external_id = ?`,
		j.SourceID, j.ExternalID).Scan(&id, &status, &favorite, &score, &research, &first, &salaryMin, &salaryMax, &currency, &fitSummary, &fitPros, &fitCons)
	if errors.Is(err, sql.ErrNoRows) {
		if j.ID == "" {
			return UpsertResult{}, fmt.Errorf("job id required")
		}
		_, err = tx.Exec(`INSERT INTO jobs(
			id, source_id, source_kind, external_id, fingerprint, title, company, location, remote, url,
			description, salary_min, salary_max, currency, employment_type, posted_at, status, favorite, score,
			filter_reason, fit_summary, fit_pros, fit_cons, research, first_seen, last_seen, updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			j.ID, j.SourceID, j.SourceKind, j.ExternalID, j.Fingerprint, j.Title, j.Company, j.Location,
			boolInt(j.Remote), j.URL, j.Description, j.SalaryMin, j.SalaryMax, j.Currency, j.EmploymentType,
			stampPtr(j.PostedAt), j.Status, boolInt(j.Favorite), j.Score, j.FilterReason, j.FitSummary, encodePoints(j.FitPros), encodePoints(j.FitCons), j.Research,
			stamp(j.FirstSeen), stamp(j.LastSeen), stamp(j.UpdatedAt))
		if err != nil {
			return UpsertResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return UpsertResult{}, err
		}
		committed = true
		return UpsertResult{ID: j.ID, Created: true}, nil
	}
	if err != nil {
		return UpsertResult{}, err
	}
	if status == model.StatusSaved || status == model.StatusDismissed {
		j.Status = status
	}
	j.Favorite = favorite == 1
	if j.Score < 0 {
		j.Score = score
		j.FitSummary = fitSummary
		j.FitPros = decodePoints(fitPros)
		j.FitCons = decodePoints(fitCons)
	} else if len(j.FitPros) == 0 && len(j.FitCons) == 0 && strings.TrimSpace(j.FitSummary) == "" {
		j.FitSummary = fitSummary
		j.FitPros = decodePoints(fitPros)
		j.FitCons = decodePoints(fitCons)
	}
	if j.Research == "" {
		j.Research = research
	}
	if j.SalaryMin == 0 && j.SalaryMax == 0 && (salaryMin > 0 || salaryMax > 0) {
		j.SalaryMin = salaryMin
		j.SalaryMax = salaryMax
		if strings.TrimSpace(j.Currency) == "" {
			j.Currency = currency
		}
	}
	_, err = tx.Exec(`UPDATE jobs SET source_kind=?, fingerprint=?, title=?, company=?, location=?, remote=?, url=?,
		description=?, salary_min=?, salary_max=?, currency=?, employment_type=?, posted_at=?, status=?, favorite=?, score=?,
		filter_reason=?, fit_summary=?, fit_pros=?, fit_cons=?, research=?, last_seen=?, updated_at=? WHERE id=?`,
		j.SourceKind, j.Fingerprint, j.Title, j.Company, j.Location, boolInt(j.Remote), j.URL, j.Description,
		j.SalaryMin, j.SalaryMax, j.Currency, j.EmploymentType, stampPtr(j.PostedAt), j.Status, boolInt(j.Favorite), j.Score,
		j.FilterReason, j.FitSummary, encodePoints(j.FitPros), encodePoints(j.FitCons), j.Research, stamp(j.LastSeen), stamp(j.UpdatedAt), id)
	if err != nil {
		return UpsertResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpsertResult{}, err
	}
	committed = true
	return UpsertResult{ID: id}, nil
}

// UpdateJobStatus sets a user-controlled status.
func (s *Store) UpdateJobStatus(id, status string) error {
	switch status {
	case model.StatusSaved, model.StatusDismissed, model.StatusMatched, model.StatusNew, model.StatusRejected:
	default:
		return fmt.Errorf("unknown status %q", status)
	}
	res, err := s.db.Exec(`UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?`, status, stamp(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetFavorite marks or clears the star on a role. A later search keeps the star.
func (s *Store) SetFavorite(id string, on bool) error {
	res, err := s.db.Exec(`UPDATE jobs SET favorite = ?, updated_at = ? WHERE id = ?`, boolInt(on), stamp(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SaveResearch writes a briefing onto a job.
func (s *Store) SaveResearch(id, research string) error {
	res, err := s.db.Exec(`UPDATE jobs SET research = ?, updated_at = ? WHERE id = ?`, research, stamp(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetJob loads one job, including description and research.
func (s *Store) GetJob(id string) (model.Job, error) {
	row := s.db.QueryRow(jobSelect+` WHERE j.id = ?`, id)
	return scanJob(row)
}

// JobFilter selects the list view.
// Sort is one of start_asc, start_desc, high_asc, high_desc. Anything else keeps the default order.
type JobFilter struct {
	Statuses  []string
	SourceIDs []string
	Query     string
	Sort      string
	Favorite  bool
	Limit     int
	Offset    int
}

// ListJobs returns summaries. Descriptions are omitted.
func (s *Store) ListJobs(f JobFilter) ([]model.Job, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	q := `SELECT j.id, j.source_id, COALESCE(s.name, ''), j.source_kind, j.external_id, j.fingerprint,
		j.title, j.company, j.location, j.remote, j.url, '', j.salary_min, j.salary_max, j.currency,
		j.employment_type, j.posted_at, j.status, j.favorite, j.score, j.filter_reason, j.fit_summary, j.fit_pros, j.fit_cons,
		CASE WHEN j.research = '' THEN '' ELSE '1' END, j.first_seen, j.last_seen, j.updated_at
		FROM jobs j LEFT JOIN sources s ON s.id = j.source_id WHERE 1=1`
	args := []any{}
	if len(f.Statuses) > 0 {
		ph := make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			ph[i] = "?"
			args = append(args, st)
		}
		q += ` AND j.status IN (` + strings.Join(ph, ",") + `)`
	}
	if f.Favorite {
		q += ` AND j.favorite = 1`
	}
	if len(f.SourceIDs) > 0 {
		ph := make([]string, len(f.SourceIDs))
		for i, id := range f.SourceIDs {
			ph[i] = "?"
			args = append(args, id)
		}
		q += ` AND j.source_id IN (` + strings.Join(ph, ",") + `)`
	}
	if needle := strings.TrimSpace(f.Query); needle != "" {
		like := "%" + escapeLike(needle) + "%"
		q += ` AND (j.title LIKE ? ESCAPE '\' OR j.company LIKE ? ESCAPE '\' OR j.location LIKE ? ESCAPE '\' OR j.fit_summary LIKE ? ESCAPE '\')`
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY ` + jobOrder(f.Sort) + ` LIMIT ? OFFSET ?`
	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		if job.Research == "1" {
			job.Research = "yes"
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

// jobOrder never interpolates the caller's string. Unknown values use the default ranking.
func jobOrder(sort string) string {
	const unknown = `(CASE WHEN j.salary_min <= 0 AND j.salary_max <= 0 THEN 1 ELSE 0 END)`
	const startPay = `(CASE WHEN j.salary_min > 0 THEN j.salary_min ELSE j.salary_max END)`
	const highPay = `(CASE WHEN j.salary_max > 0 THEN j.salary_max ELSE j.salary_min END)`
	switch strings.TrimSpace(sort) {
	case "start_asc":
		return unknown + ", " + startPay + " ASC, j.title COLLATE NOCASE ASC"
	case "start_desc":
		return unknown + ", " + startPay + " DESC, j.title COLLATE NOCASE ASC"
	case "high_asc":
		return unknown + ", " + highPay + " ASC, j.title COLLATE NOCASE ASC"
	case "high_desc":
		return unknown + ", " + highPay + " DESC, j.title COLLATE NOCASE ASC"
	default:
		return `CASE j.status WHEN 'saved' THEN 0 WHEN 'matched' THEN 1 WHEN 'new' THEN 2 ELSE 3 END, j.score DESC, j.last_seen DESC`
	}
}

// BackfillSalaries fills missing pay from posting text.
// It also repairs European thousands that were stored as 68 instead of 68000,
// and drops a small lone amount that is no longer recognized as pay.
func (s *Store) BackfillSalaries() (int, error) {
	rows, err := s.db.Query(`SELECT id, description, salary_min, salary_max, currency FROM jobs WHERE description != ''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type hit struct {
		id, currency string
		min, max     int
	}
	var hits []hit
	for rows.Next() {
		var id, desc, cur string
		var haveMin, haveMax int
		if err := rows.Scan(&id, &desc, &haveMin, &haveMax, &cur); err != nil {
			return 0, err
		}
		min, max, parsed, ok := textutil.ParseSalary(desc)
		switch {
		case ok && haveMin == 0 && haveMax == 0:
		case ok && haveMax > 0 && haveMax < 1000 && min >= haveMax*50:
		case !ok && haveMin > 0 && haveMin == haveMax && haveMax < 20000 && moneyMention(desc, haveMin):
			min, max, parsed = 0, 0, ""
		default:
			continue
		}
		if strings.TrimSpace(cur) == "" {
			cur = parsed
		}
		if min == 0 && max == 0 {
			cur = ""
		}
		hits = append(hits, hit{id: id, currency: cur, min: min, max: max})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range hits {
		if _, err := s.db.Exec(`UPDATE jobs SET salary_min = ?, salary_max = ?, currency = ? WHERE id = ?`,
			item.min, item.max, item.currency, item.id); err != nil {
			return 0, err
		}
	}
	return len(hits), nil
}

// Refilter applies the current profile to stored roles.
// Saved and dismissed roles keep their status.
func (s *Store) Refilter(profile model.Profile) (int, error) {
	rows, err := s.db.Query(jobSelect)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var jobs []model.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	now := stamp(time.Now().UTC())
	for _, job := range jobs {
		if job.Status == model.StatusSaved || job.Status == model.StatusDismissed {
			continue
		}
		decision := filter.Decide(profile, job)
		status := model.StatusMatched
		reason := decision.Reason
		if !decision.Pass {
			status = model.StatusRejected
		} else if job.Score >= 0 && profile.MinScore > 0 && job.Score < profile.MinScore {
			status = model.StatusRejected
			reason = fmt.Sprintf("score %d is below %d", job.Score, profile.MinScore)
		}
		if status == job.Status && reason == job.FilterReason {
			continue
		}
		if _, err := s.db.Exec(`UPDATE jobs SET status = ?, filter_reason = ?, updated_at = ? WHERE id = ?`, status, reason, now, job.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// BackfillFitPoints writes plus and minus reasons for matches that do not have them yet.
func (s *Store) BackfillFitPoints(profile model.Profile) (int, error) {
	rows, err := s.db.Query(jobSelect + ` WHERE j.status IN ('matched', 'saved') AND j.fit_pros = '' AND j.fit_cons = ''`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var jobs []model.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, job := range jobs {
		_, _, parsed := filter.ParseSummary(job.FitSummary)
		pros, cons := filter.PointsFor(profile, job)
		if len(pros) == 0 && len(cons) == 0 {
			continue
		}
		summary := job.FitSummary
		if strings.TrimSpace(summary) == "" || parsed {
			summary = filter.FormatFit(pros, cons)
		}
		res, err := s.db.Exec(`UPDATE jobs SET fit_summary = ?, fit_pros = ?, fit_cons = ? WHERE id = ? AND fit_pros = '' AND fit_cons = ''`,
			summary, encodePoints(pros), encodePoints(cons), job.ID)
		if err != nil {
			return n, err
		}
		changed, _ := res.RowsAffected()
		n += int(changed)
	}
	return n, nil
}

func encodePoints(items []string) string {
	if len(items) == 0 {
		return ""
	}
	b, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodePoints(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

func moneyMention(desc string, amount int) bool {
	if amount <= 0 {
		return false
	}
	tokens := []string{strconv.Itoa(amount)}
	if grouped := groupThousands(amount); grouped != "" {
		tokens = append(tokens, grouped)
	}
	for _, token := range tokens {
		from := 0
		for {
			at := strings.Index(desc[from:], token)
			if at < 0 {
				break
			}
			at += from
			if currencyNear(desc, at) {
				return true
			}
			from = at + len(token)
		}
	}
	return false
}

func currencyNear(desc string, at int) bool {
	start := at - 6
	if start < 0 {
		start = 0
	}
	window := strings.ToUpper(desc[start:at])
	return strings.ContainsAny(window, "$£€") || strings.Contains(window, "USD") || strings.Contains(window, "EUR") || strings.Contains(window, "GBP")
}

func groupThousands(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return ""
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

const jobSelect = `SELECT j.id, j.source_id, COALESCE(s.name, ''), j.source_kind, j.external_id, j.fingerprint,
	j.title, j.company, j.location, j.remote, j.url, j.description, j.salary_min, j.salary_max, j.currency,
	j.employment_type, j.posted_at, j.status, j.favorite, j.score, j.filter_reason, j.fit_summary, j.fit_pros, j.fit_cons, j.research,
	j.first_seen, j.last_seen, j.updated_at
	FROM jobs j LEFT JOIN sources s ON s.id = j.source_id`

func scanJob(sc scanner) (model.Job, error) {
	var j model.Job
	var remote, favorite int
	var posted, first, last, updated, fitPros, fitCons string
	if err := sc.Scan(&j.ID, &j.SourceID, &j.SourceName, &j.SourceKind, &j.ExternalID, &j.Fingerprint,
		&j.Title, &j.Company, &j.Location, &remote, &j.URL, &j.Description, &j.SalaryMin, &j.SalaryMax,
		&j.Currency, &j.EmploymentType, &posted, &j.Status, &favorite, &j.Score, &j.FilterReason, &j.FitSummary,
		&fitPros, &fitCons, &j.Research, &first, &last, &updated); err != nil {
		return j, err
	}
	j.Favorite = favorite == 1
	j.FitPros = decodePoints(fitPros)
	j.FitCons = decodePoints(fitCons)
	j.Remote = remote == 1
	if posted != "" {
		t := parseStamp(posted)
		if !t.IsZero() {
			j.PostedAt = &t
		}
	}
	j.FirstSeen = parseStamp(first)
	j.LastSeen = parseStamp(last)
	j.UpdatedAt = parseStamp(updated)
	return j, nil
}

// JobsForContext returns saved and matched roles, highest score first.
func (s *Store) JobsForContext(limit int) ([]model.Job, error) {
	if limit <= 0 {
		limit = 80
	}
	rows, err := s.db.Query(jobSelect+` WHERE j.status IN ('saved','matched') OR j.favorite = 1 ORDER BY CASE WHEN j.status = 'saved' OR j.favorite = 1 THEN 0 ELSE 1 END, j.score DESC, j.last_seen DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

// CountByStatus returns job totals keyed by status.
func (s *Store) CountByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(1) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var favorites int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM jobs WHERE favorite = 1`).Scan(&favorites); err != nil {
		return nil, err
	}
	out["favorite"] = favorites
	var purgeable int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM jobs WHERE status != ? AND favorite = 0`, model.StatusSaved).Scan(&purgeable); err != nil {
		return nil, err
	}
	out["purgeable"] = purgeable
	return out, nil
}

// PurgeUnkept deletes roles that are neither saved nor favorited.
func (s *Store) PurgeUnkept() (int, error) {
	res, err := s.db.Exec(`DELETE FROM jobs WHERE status != ? AND favorite = 0`, model.StatusSaved)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// JobIDs returns every role id still stored.
func (s *Store) JobIDs() ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FailRunning marks leftover running rows after a restart.
func (s *Store) FailRunning(msg string) error {
	_, err := s.db.Exec(`UPDATE runs SET status = ?, finished_at = ?, message = ? WHERE status = ?`,
		model.RunError, stamp(time.Now().UTC()), msg, model.RunRunning)
	return err
}

// InsertRun creates a run row.
func (s *Store) InsertRun(r model.Run) error {
	_, err := s.db.Exec(`INSERT INTO runs(id, trigger, status, started_at, finished_at, fetched, new_count, matched, rejected, message)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, r.ID, r.Trigger, r.Status, stamp(r.StartedAt), stampPtr(r.FinishedAt), r.Fetched, r.New, r.Matched, r.Rejected, r.Message)
	return err
}

// UpdateRun writes progress onto an existing run.
func (s *Store) UpdateRun(r model.Run) error {
	_, err := s.db.Exec(`UPDATE runs SET status=?, finished_at=?, fetched=?, new_count=?, matched=?, rejected=?, message=? WHERE id=?`,
		r.Status, stampPtr(r.FinishedAt), r.Fetched, r.New, r.Matched, r.Rejected, r.Message, r.ID)
	return err
}

// GetRun loads one run.
func (s *Store) GetRun(id string) (model.Run, error) {
	row := s.db.QueryRow(`SELECT id, trigger, status, started_at, finished_at, fetched, new_count, matched, rejected, message FROM runs WHERE id = ?`, id)
	return scanRun(row)
}

// ActiveRun returns the newest running run, if any.
func (s *Store) ActiveRun() (model.Run, bool, error) {
	row := s.db.QueryRow(`SELECT id, trigger, status, started_at, finished_at, fetched, new_count, matched, rejected, message FROM runs WHERE status = ? ORDER BY started_at DESC LIMIT 1`, model.RunRunning)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Run{}, false, nil
	}
	return r, err == nil, err
}

// LatestRun returns the newest run.
func (s *Store) LatestRun() (model.Run, bool, error) {
	row := s.db.QueryRow(`SELECT id, trigger, status, started_at, finished_at, fetched, new_count, matched, rejected, message FROM runs ORDER BY started_at DESC LIMIT 1`)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Run{}, false, nil
	}
	return r, err == nil, err
}

// ListRuns returns recent runs.
func (s *Store) ListRuns(limit int) ([]model.Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id, trigger, status, started_at, finished_at, fetched, new_count, matched, rejected, message FROM runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanRun(sc scanner) (model.Run, error) {
	var r model.Run
	var started, finished string
	if err := sc.Scan(&r.ID, &r.Trigger, &r.Status, &started, &finished, &r.Fetched, &r.New, &r.Matched, &r.Rejected, &r.Message); err != nil {
		return r, err
	}
	r.StartedAt = parseStamp(started)
	if finished != "" {
		t := parseStamp(finished)
		r.FinishedAt = &t
	}
	return r, nil
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func stampPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return stamp(*t)
}

func parseStamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

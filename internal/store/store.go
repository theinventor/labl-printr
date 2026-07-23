// Package store is labl-printr's SQLite persistence: printers, custom
// templates, job history, and the virtual printer's output tray.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // modernc sqlite prefers a single writer
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS printers (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  kind        TEXT NOT NULL DEFAULT 'network',
  serial      TEXT,
  host        TEXT,
  port        INTEGER NOT NULL DEFAULT 9100,
  dpmm        INTEGER NOT NULL DEFAULT 8,
  width_dots  INTEGER NOT NULL DEFAULT 487,
  left_shift  INTEGER NOT NULL DEFAULT 0,
  is_default  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS custom_templates (
  id          INTEGER PRIMARY KEY,
  slug        TEXT NOT NULL UNIQUE,
  name        TEXT NOT NULL,
  zpl         TEXT NOT NULL,
  width_dots  INTEGER NOT NULL,
  length_dots INTEGER NOT NULL,
  dpmm        INTEGER NOT NULL DEFAULT 8,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS jobs (
  id              INTEGER PRIMARY KEY,
  printer_id      INTEGER NOT NULL REFERENCES printers(id),
  template_id     TEXT,
  vars            TEXT,
  zpl             TEXT NOT NULL,
  width_dots      INTEGER NOT NULL,
  length_dots     INTEGER NOT NULL,
  copies          INTEGER NOT NULL DEFAULT 1,
  state           TEXT NOT NULL DEFAULT 'queued',
  error           TEXT,
  idempotency_key TEXT UNIQUE,
  source          TEXT NOT NULL DEFAULT 'api',
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS virtual_prints (
  id         INTEGER PRIMARY KEY,
  job_id     INTEGER,
  zpl        TEXT NOT NULL,
  png        BLOB NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);`)
	return err
}

// ---- Printers

const (
	KindNetwork = "network"
	KindVirtual = "virtual"
)

type Printer struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // network | virtual
	Serial    string `json:"serial,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port"`
	Dpmm      int    `json:"dpmm"`
	WidthDots int    `json:"widthDots"`
	LeftShift int    `json:"leftShift"`
	IsDefault bool   `json:"isDefault"`
	CreatedAt string `json:"createdAt"`
}

func scanPrinter(row interface{ Scan(...any) error }) (Printer, error) {
	var p Printer
	var serial, host sql.NullString
	var def int
	err := row.Scan(&p.ID, &p.Name, &p.Kind, &serial, &host, &p.Port, &p.Dpmm, &p.WidthDots, &p.LeftShift, &def, &p.CreatedAt)
	p.Serial, p.Host, p.IsDefault = serial.String, host.String, def == 1
	return p, err
}

const printerCols = "id, name, kind, serial, host, port, dpmm, width_dots, left_shift, is_default, created_at"

func (s *Store) Printers() ([]Printer, error) {
	rows, err := s.db.Query("SELECT " + printerCols + " FROM printers ORDER BY is_default DESC, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Printer
	for rows.Next() {
		p, err := scanPrinter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Printer(id int64) (Printer, error) {
	return scanPrinter(s.db.QueryRow("SELECT "+printerCols+" FROM printers WHERE id = ?", id))
}

func (s *Store) DefaultPrinter() (Printer, error) {
	p, err := scanPrinter(s.db.QueryRow("SELECT " + printerCols + " FROM printers WHERE is_default = 1"))
	if errors.Is(err, sql.ErrNoRows) {
		return scanPrinter(s.db.QueryRow("SELECT " + printerCols + " FROM printers ORDER BY id LIMIT 1"))
	}
	return p, err
}

func (s *Store) CreatePrinter(p Printer) (Printer, error) {
	res, err := s.db.Exec(`INSERT INTO printers (name, kind, serial, host, port, dpmm, width_dots, left_shift, is_default)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		p.Name, p.Kind, nullable(p.Serial), nullable(p.Host), p.Port, p.Dpmm, p.WidthDots, p.LeftShift, boolInt(p.IsDefault))
	if err != nil {
		return Printer{}, err
	}
	id, _ := res.LastInsertId()
	if p.IsDefault {
		if err := s.SetDefaultPrinter(id); err != nil {
			return Printer{}, err
		}
	}
	return s.Printer(id)
}

func (s *Store) SetDefaultPrinter(id int64) error {
	if _, err := s.db.Exec("UPDATE printers SET is_default = CASE WHEN id = ? THEN 1 ELSE 0 END", id); err != nil {
		return err
	}
	return nil
}

func (s *Store) DeletePrinter(id int64) error {
	_, err := s.db.Exec("DELETE FROM printers WHERE id = ?", id)
	return err
}

func (s *Store) PrinterByName(name string) (Printer, error) {
	return scanPrinter(s.db.QueryRow("SELECT "+printerCols+" FROM printers WHERE name = ?", name))
}

// ---- Custom templates

type CustomTemplate struct {
	ID         int64  `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	ZPL        string `json:"zpl"`
	WidthDots  int    `json:"widthDots"`
	LengthDots int    `json:"lengthDots"`
	Dpmm       int    `json:"dpmm"`
	CreatedAt  string `json:"createdAt"`
}

func (s *Store) CustomTemplates() ([]CustomTemplate, error) {
	rows, err := s.db.Query("SELECT id, slug, name, zpl, width_dots, length_dots, dpmm, created_at FROM custom_templates ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomTemplate
	for rows.Next() {
		var t CustomTemplate
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.ZPL, &t.WidthDots, &t.LengthDots, &t.Dpmm, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CustomTemplateBySlug(slug string) (CustomTemplate, error) {
	var t CustomTemplate
	err := s.db.QueryRow("SELECT id, slug, name, zpl, width_dots, length_dots, dpmm, created_at FROM custom_templates WHERE slug = ?", slug).
		Scan(&t.ID, &t.Slug, &t.Name, &t.ZPL, &t.WidthDots, &t.LengthDots, &t.Dpmm, &t.CreatedAt)
	return t, err
}

func (s *Store) SaveCustomTemplate(t CustomTemplate) (CustomTemplate, error) {
	_, err := s.db.Exec(`INSERT INTO custom_templates (slug, name, zpl, width_dots, length_dots, dpmm)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(slug) DO UPDATE SET name = excluded.name, zpl = excluded.zpl,
		  width_dots = excluded.width_dots, length_dots = excluded.length_dots, dpmm = excluded.dpmm`,
		t.Slug, t.Name, t.ZPL, t.WidthDots, t.LengthDots, t.Dpmm)
	if err != nil {
		return CustomTemplate{}, err
	}
	return s.CustomTemplateBySlug(t.Slug)
}

func (s *Store) DeleteCustomTemplate(slug string) error {
	_, err := s.db.Exec("DELETE FROM custom_templates WHERE slug = ?", slug)
	return err
}

// ---- Jobs

const (
	JobQueued   = "queued"
	JobPrinting = "printing"
	JobDone     = "done"
	JobFailed   = "failed"
	JobCanceled = "canceled"
)

type Job struct {
	ID             int64             `json:"id"`
	PrinterID      int64             `json:"printerId"`
	PrinterName    string            `json:"printerName,omitempty"`
	TemplateID     string            `json:"templateId,omitempty"`
	Vars           map[string]string `json:"vars,omitempty"`
	ZPL            string            `json:"-"`
	WidthDots      int               `json:"widthDots"`
	LengthDots     int               `json:"lengthDots"`
	Copies         int               `json:"copies"`
	State          string            `json:"state"`
	Error          string            `json:"error,omitempty"`
	IdempotencyKey string            `json:"-"`
	Source         string            `json:"source"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
}

func (s *Store) CreateJob(j Job) (Job, bool, error) {
	if j.IdempotencyKey != "" {
		existing, err := s.JobByIdempotencyKey(j.IdempotencyKey)
		if err == nil {
			return existing, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Job{}, false, err
		}
	}
	vars, _ := json.Marshal(j.Vars)
	res, err := s.db.Exec(`INSERT INTO jobs (printer_id, template_id, vars, zpl, width_dots, length_dots, copies, state, idempotency_key, source)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		j.PrinterID, nullable(j.TemplateID), string(vars), j.ZPL, j.WidthDots, j.LengthDots, j.Copies, JobQueued, nullable(j.IdempotencyKey), j.Source)
	if err != nil {
		// Two submits raced on the same idempotency key: the UNIQUE index
		// rejected ours, so the winner's job is the caller's job.
		if j.IdempotencyKey != "" && strings.Contains(err.Error(), "UNIQUE") {
			if existing, qerr := s.JobByIdempotencyKey(j.IdempotencyKey); qerr == nil {
				return existing, true, nil
			}
		}
		return Job{}, false, err
	}
	id, _ := res.LastInsertId()
	s.PruneJobs()
	created, err := s.Job(id)
	return created, false, err
}

const jobCols = `j.id, j.printer_id, COALESCE(p.name,''), j.template_id, j.vars, j.zpl, j.width_dots, j.length_dots,
	j.copies, j.state, j.error, j.idempotency_key, j.source, j.created_at, j.updated_at`

// jobColsList swaps the ZPL payload for an empty literal: list endpoints never
// serialize it, and raw jobs can be megabytes each.
const jobColsList = `j.id, j.printer_id, COALESCE(p.name,''), j.template_id, j.vars, '', j.width_dots, j.length_dots,
	j.copies, j.state, j.error, j.idempotency_key, j.source, j.created_at, j.updated_at`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var tpl, vars, jerr, ikey sql.NullString
	err := row.Scan(&j.ID, &j.PrinterID, &j.PrinterName, &tpl, &vars, &j.ZPL, &j.WidthDots, &j.LengthDots,
		&j.Copies, &j.State, &jerr, &ikey, &j.Source, &j.CreatedAt, &j.UpdatedAt)
	j.TemplateID, j.Error, j.IdempotencyKey = tpl.String, jerr.String, ikey.String
	if vars.Valid && vars.String != "" && vars.String != "null" {
		_ = json.Unmarshal([]byte(vars.String), &j.Vars)
	}
	return j, err
}

func (s *Store) Job(id int64) (Job, error) {
	return scanJob(s.db.QueryRow("SELECT "+jobCols+" FROM jobs j LEFT JOIN printers p ON p.id = j.printer_id WHERE j.id = ?", id))
}

func (s *Store) JobByIdempotencyKey(key string) (Job, error) {
	return scanJob(s.db.QueryRow("SELECT "+jobCols+" FROM jobs j LEFT JOIN printers p ON p.id = j.printer_id WHERE j.idempotency_key = ?", key))
}

func (s *Store) Jobs(limit int) ([]Job, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query("SELECT "+jobColsList+" FROM jobs j LEFT JOIN printers p ON p.id = j.printer_id ORDER BY j.id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) QueuedJobs() ([]Job, error) {
	rows, err := s.db.Query("SELECT " + jobColsList + " FROM jobs j LEFT JOIN printers p ON p.id = j.printer_id WHERE j.state IN ('queued','printing') ORDER BY j.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) SetJobState(id int64, state, errMsg string) error {
	_, err := s.db.Exec("UPDATE jobs SET state = ?, error = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?",
		state, nullable(errMsg), id)
	return err
}

// ---- Virtual prints

type VirtualPrint struct {
	ID        int64  `json:"id"`
	JobID     *int64 `json:"jobId,omitempty"`
	CreatedAt string `json:"createdAt"`
}

func (s *Store) AddVirtualPrint(jobID *int64, zpl string, png []byte) error {
	_, err := s.db.Exec("INSERT INTO virtual_prints (job_id, zpl, png) VALUES (?,?,?)", jobID, zpl, png)
	if err == nil {
		s.pruneVirtualPrints()
	}
	return err
}

func (s *Store) VirtualPrints(limit int) ([]VirtualPrint, error) {
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	rows, err := s.db.Query("SELECT id, job_id, created_at FROM virtual_prints ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VirtualPrint
	for rows.Next() {
		var v VirtualPrint
		var jobID sql.NullInt64
		if err := rows.Scan(&v.ID, &jobID, &v.CreatedAt); err != nil {
			return nil, err
		}
		if jobID.Valid {
			v.JobID = &jobID.Int64
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) VirtualPrintPNG(id int64) ([]byte, error) {
	var png []byte
	err := s.db.QueryRow("SELECT png FROM virtual_prints WHERE id = ?", id).Scan(&png)
	return png, err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// pruneVirtualPrints keeps the tray from growing without bound.
const trayKeep = 400

func (s *Store) pruneVirtualPrints() {
	if _, err := s.db.Exec(`DELETE FROM virtual_prints WHERE id NOT IN
		(SELECT id FROM virtual_prints ORDER BY id DESC LIMIT ?)`, trayKeep); err != nil {
		log.Printf("store: tray prune failed: %v", err)
	}
}

// PruneJobs keeps history browsable but bounded — each row carries its full
// ZPL for reprint, so an immortal jobs table grows by the payload size on
// every print.
const jobsKeep = 2000

func (s *Store) PruneJobs() {
	if _, err := s.db.Exec(`DELETE FROM jobs WHERE id NOT IN
		(SELECT id FROM jobs ORDER BY id DESC LIMIT ?)`, jobsKeep); err != nil {
		log.Printf("store: jobs prune failed: %v", err)
	}
}

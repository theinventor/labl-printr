// Package server is the labl-printr HTTP surface: REST API + embedded web UI
// + embedded ZebraPrintLab designer, one binary, one port.
package server

import (
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/theinventor/labl-printr/internal/jobs"
	"github.com/theinventor/labl-printr/internal/labels"
	"github.com/theinventor/labl-printr/internal/printer"
	"github.com/theinventor/labl-printr/internal/render"
	"github.com/theinventor/labl-printr/internal/store"
	"github.com/theinventor/labl-printr/internal/templates"
)

//go:embed all:dist
var webDist embed.FS

//go:embed all:designerdist
var designerDist embed.FS

type Server struct {
	Store   *store.Store
	Jobs    *jobs.Manager
	Virtual *printer.Virtual
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req.Body = http.MaxBytesReader(w, req.Body, 8<<20)
			next.ServeHTTP(w, req)
		})
	})

	r.Route("/api", func(api chi.Router) {
		api.Get("/templates", s.listTemplates)
		api.Get("/templates/{id}", s.getTemplate)
		api.Delete("/templates/{id}", s.deleteTemplate)
		api.Post("/preview", s.preview)
		api.Post("/jobs", s.createJob)
		api.Get("/jobs", s.listJobs)
		api.Get("/jobs/{id}", s.getJob)
		api.Get("/jobs/{id}/preview.png", s.jobPreview)
		api.Post("/jobs/{id}/reprint", s.reprint)
		api.Get("/printers", s.listPrinters)
		api.Post("/printers", s.createPrinter)
		api.Delete("/printers/{id}", s.deletePrinter)
		api.Post("/printers/{id}/default", s.setDefaultPrinter)
		api.Get("/printers/{id}/status", s.printerStatus)
		api.Post("/printers/discover", s.discover)
		api.Post("/designer-import", s.designerImport)
		api.Get("/tray", s.listTray)
		api.Get("/tray/{id}.png", s.trayPNG)
	})

	r.Handle("/designer", http.RedirectHandler("/designer/", http.StatusMovedPermanently))
	designerFS, _ := fs.Sub(designerDist, "designerdist")
	r.Handle("/designer/*", http.StripPrefix("/designer/", spaHandler(designerFS)))

	webFS, _ := fs.Sub(webDist, "dist")
	r.Handle("/*", spaHandler(webFS))
	return r
}

// spaHandler serves static files, falling back to index.html for client-side
// routes.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(fsys, path); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

// ---- helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func (s *Server) resolveTemplate(id string) (*templates.Template, templates.Profile, error) {
	if t, ok := templates.Get(id); ok {
		return t, templates.DefaultProfile, nil
	}
	ct, err := s.Store.CustomTemplateBySlug(id)
	if err != nil {
		return nil, templates.Profile{}, fmt.Errorf("unknown template %q", id)
	}
	t := templates.CustomTemplate(ct.Slug, ct.Name, ct.ZPL, ct.LengthDots)
	return t, templates.Profile{Dpmm: ct.Dpmm, WidthDots: ct.WidthDots}, nil
}

func (s *Server) resolvePrinter(id int64, name string) (store.Printer, error) {
	if id > 0 {
		return s.Store.Printer(id)
	}
	if name != "" {
		return s.Store.PrinterByName(name)
	}
	return s.Store.DefaultPrinter()
}

// profileFor overlays a printer's physical geometry onto template rendering.
func profileFor(p store.Printer) templates.Profile {
	return templates.Profile{Dpmm: p.Dpmm, WidthDots: p.WidthDots, LeftShift: p.LeftShift}
}

// ---- templates

type templateJSON struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Fields      []templates.Field `json:"fields"`
	Builtin     bool              `json:"builtin"`
}

func toJSON(t *templates.Template) templateJSON {
	return templateJSON{ID: t.ID, Name: t.Name, Description: t.Description, Fields: t.Fields, Builtin: t.Builtin}
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	var out []templateJSON
	for _, t := range templates.Builtins() {
		out = append(out, toJSON(t))
	}
	customs, err := s.Store.CustomTemplates()
	if err != nil {
		writeErr(w, 500, "load custom templates: %v", err)
		return
	}
	for _, ct := range customs {
		out = append(out, toJSON(templates.CustomTemplate(ct.Slug, ct.Name, ct.ZPL, ct.LengthDots)))
	}
	writeJSON(w, 200, out)
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	t, _, err := s.resolveTemplate(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, 404, "%v", err)
		return
	}
	writeJSON(w, 200, toJSON(t))
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := templates.Get(id); ok {
		writeErr(w, 400, "built-in templates can't be deleted")
		return
	}
	if err := s.Store.DeleteCustomTemplate(id); err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}

// ---- preview

type previewReq struct {
	TemplateID  string            `json:"templateId"`
	Vars        map[string]string `json:"vars"`
	PrinterID   int64             `json:"printerId"`
	PrinterName string            `json:"printer"`
	Copies      int               `json:"copies"`
	ZPL         string            `json:"zpl"`
}

type previewResp struct {
	PNG        string `json:"png"` // base64
	ZPL        string `json:"zpl"`
	WidthDots  int    `json:"widthDots"`
	LengthDots int    `json:"lengthDots"`
	Dpmm       int    `json:"dpmm"`
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var req previewReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json: %v", err)
		return
	}
	f, dpmm, err := s.finalize(req)
	if err != nil {
		writeErr(w, 422, "%v", err)
		return
	}
	writeJSON(w, 200, previewResp{
		PNG:        base64.StdEncoding.EncodeToString(f.PNG),
		ZPL:        f.ZPL,
		WidthDots:  f.WidthDots,
		LengthDots: f.LengthDots,
		Dpmm:       dpmm,
	})
}

func (s *Server) finalize(req previewReq) (labels.Final, int, error) {
	if req.ZPL != "" {
		wDots, lDots := labels.Dims(req.ZPL)
		dpmm := templates.DefaultProfile.Dpmm
		png, err := render.PNG(req.ZPL, wDots, lDots, dpmm)
		if err != nil {
			return labels.Final{}, 0, err
		}
		return labels.Final{ZPL: req.ZPL, WidthDots: wDots, LengthDots: lDots, PNG: png}, dpmm, nil
	}
	t, profile, err := s.resolveTemplate(req.TemplateID)
	if err != nil {
		return labels.Final{}, 0, err
	}
	if p, perr := s.resolvePrinter(req.PrinterID, req.PrinterName); perr == nil && t.Builtin {
		profile = profileFor(p)
	}
	f, err := labels.Finalize(t, req.Vars, profile, req.Copies)
	return f, profile.Dpmm, err
}

// ---- jobs

type jobReq struct {
	previewReq
	IdempotencyKey string `json:"idempotencyKey"`
	Source         string `json:"source"`
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req jobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json: %v", err)
		return
	}
	// Idempotent replays return the existing job before any render work.
	if req.IdempotencyKey != "" {
		if existing, err := s.Store.JobByIdempotencyKey(req.IdempotencyKey); err == nil {
			writeJSON(w, 200, existing)
			return
		}
	}
	p, err := s.resolvePrinter(req.PrinterID, req.PrinterName)
	if err != nil {
		writeErr(w, 422, "no printer available: %v", err)
		return
	}
	if req.Copies < 1 {
		req.Copies = 1
	}
	if req.Copies > 100 {
		writeErr(w, 422, "copies capped at 100 (asked for %d)", req.Copies)
		return
	}
	f, _, err := s.finalize(req.previewReq)
	if err != nil {
		writeErr(w, 422, "%v", err)
		return
	}
	if req.Source == "" {
		req.Source = "api"
	}
	job, existed, err := s.Store.CreateJob(store.Job{
		PrinterID:      p.ID,
		TemplateID:     req.TemplateID,
		Vars:           req.Vars,
		ZPL:            f.ZPL,
		WidthDots:      f.WidthDots,
		LengthDots:     f.LengthDots,
		Copies:         req.Copies,
		IdempotencyKey: req.IdempotencyKey,
		Source:         req.Source,
	})
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if !existed && !s.Jobs.Enqueue(job) {
		// Queue overflow: the job row is already marked failed — report that
		// honestly instead of a phantom "queued".
		if reloaded, rerr := s.Store.Job(job.ID); rerr == nil {
			job = reloaded
		}
		writeJSON(w, 503, job)
		return
	}
	status := 201
	if existed {
		status = 200
	}
	writeJSON(w, status, job)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Store.Jobs(limit)
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if list == nil {
		list = []store.Job{}
	}
	writeJSON(w, 200, list)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	j, err := s.Store.Job(id)
	if err != nil {
		writeErr(w, 404, "job not found")
		return
	}
	writeJSON(w, 200, j)
}

func (s *Server) jobPreview(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	j, err := s.Store.Job(id)
	if err != nil {
		writeErr(w, 404, "job not found")
		return
	}
	dpmm := 8
	if p, perr := s.Store.Printer(j.PrinterID); perr == nil && p.Dpmm > 0 {
		dpmm = p.Dpmm
	}
	png, err := render.PNG(j.ZPL, j.WidthDots, j.LengthDots, dpmm)
	if err != nil {
		writeErr(w, 500, "render: %v", err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(png)
}

func (s *Server) reprint(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	j, err := s.Store.Job(id)
	if err != nil {
		writeErr(w, 404, "job not found")
		return
	}
	clone, _, err := s.Store.CreateJob(store.Job{
		PrinterID: j.PrinterID, TemplateID: j.TemplateID, Vars: j.Vars,
		ZPL: j.ZPL, WidthDots: j.WidthDots, LengthDots: j.LengthDots,
		Copies: j.Copies, Source: "reprint",
	})
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	s.Jobs.Enqueue(clone)
	writeJSON(w, 201, clone)
}

// ---- printers

func (s *Server) listPrinters(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Printers()
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if list == nil {
		list = []store.Printer{}
	}
	writeJSON(w, 200, list)
}

func (s *Server) createPrinter(w http.ResponseWriter, r *http.Request) {
	var p store.Printer
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, 400, "bad json: %v", err)
		return
	}
	if p.Name == "" || (p.Kind != store.KindVirtual && p.Host == "") {
		writeErr(w, 422, "name and host are required")
		return
	}
	if p.Kind == "" {
		p.Kind = store.KindNetwork
	}
	if p.Port == 0 {
		p.Port = 9100
	}
	if p.Dpmm == 0 {
		p.Dpmm = 8
	}
	// Per-dpi geometry for 2.4" media lives here, not in the web form, so
	// printers added via CLI or curl get the same dot math and the ZD-series
	// narrow-media centering shift.
	if p.WidthDots == 0 {
		p.WidthDots = 487
		if p.Dpmm == 12 {
			p.WidthDots = 720
		}
	}
	if p.LeftShift == 0 && p.Kind == store.KindNetwork {
		p.LeftShift = 172
		if p.Dpmm == 12 {
			p.LeftShift = 280
		}
	}
	created, err := s.Store.CreatePrinter(p)
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 201, created)
}

func (s *Server) deletePrinter(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := s.Store.Printer(id)
	if err != nil {
		writeErr(w, 404, "printer not found")
		return
	}
	if p.Kind == store.KindVirtual {
		writeErr(w, 400, "the virtual printer can't be deleted")
		return
	}
	if err := s.Store.DeletePrinter(id); err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}

func (s *Server) setDefaultPrinter(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Store.Printer(id); err != nil {
		// Guard before the UPDATE: a bad id would otherwise clear every
		// default and silently shift printing to the lowest-id printer.
		writeErr(w, 404, "printer not found")
		return
	}
	if err := s.Store.SetDefaultPrinter(id); err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) printerStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	p, err := s.Store.Printer(id)
	if err != nil {
		writeErr(w, 404, "printer not found")
		return
	}
	writeJSON(w, 200, s.Jobs.PrinterStatus(p))
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	found, err := printer.Discover(2500 * time.Millisecond)
	if err != nil {
		writeErr(w, 500, "discovery failed: %v", err)
		return
	}
	if found == nil {
		found = []printer.Discovered{}
	}
	writeJSON(w, 200, found)
}

// ---- designer import

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

type designerImportReq struct {
	Name       string `json:"name"`
	ZPL        string `json:"zpl"`
	WidthDots  int    `json:"widthDots"`
	HeightDots int    `json:"heightDots"`
	Dpmm       int    `json:"dpmm"`
}

func (s *Server) designerImport(w http.ResponseWriter, r *http.Request) {
	var req designerImportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json: %v", err)
		return
	}
	if strings.TrimSpace(req.ZPL) == "" {
		writeErr(w, 422, "zpl is required")
		return
	}
	if req.Name == "" {
		req.Name = "Designer label"
	}
	if req.Dpmm == 0 {
		req.Dpmm = 8
	}
	if req.WidthDots == 0 || req.HeightDots == 0 {
		req.WidthDots, req.HeightDots = labels.Dims(req.ZPL)
	}
	if req.WidthDots > labels.MaxWidthDots || req.HeightDots > labels.MaxLengthDots {
		writeErr(w, 422, "label dimensions %dx%d dots exceed hardware-plausible bounds", req.WidthDots, req.HeightDots)
		return
	}
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(req.Name), "-"), "-")
	if slug == "" {
		slug = "designer-label"
	}
	// Uniquify against builtins and existing customs so an import never
	// silently overwrites a different label. Re-importing the same name is an
	// intentional update (upsert).
	if _, isBuiltin := templates.Get(slug); isBuiltin {
		slug += "-custom"
	}
	base := slug
	for i := 2; ; i++ {
		existing, err := s.Store.CustomTemplateBySlug(slug)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && existing.Name == req.Name) {
			break
		}
		if err != nil {
			writeErr(w, 500, "%v", err)
			return
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	saved, err := s.Store.SaveCustomTemplate(store.CustomTemplate{
		Slug: slug, Name: req.Name, ZPL: req.ZPL,
		WidthDots: req.WidthDots, LengthDots: req.HeightDots, Dpmm: req.Dpmm,
	})
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	log.Printf("designer import: saved template %q (%dx%d)", saved.Slug, saved.WidthDots, saved.LengthDots)
	writeJSON(w, 201, saved)
}

// ---- virtual tray

func (s *Server) listTray(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Store.VirtualPrints(limit)
	if err != nil {
		writeErr(w, 500, "%v", err)
		return
	}
	if list == nil {
		list = []store.VirtualPrint{}
	}
	writeJSON(w, 200, list)
}

func (s *Server) trayPNG(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	png, err := s.Store.VirtualPrintPNG(id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(png)
}

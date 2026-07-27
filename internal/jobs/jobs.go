// Package jobs runs the per-printer print queues: one worker goroutine per
// printer, so jobs to the same device serialize while different printers
// print concurrently.
package jobs

import (
	"log"
	"sync"

	"github.com/theinventor/labl-printr/internal/brother"
	"github.com/theinventor/labl-printr/internal/printer"
	"github.com/theinventor/labl-printr/internal/render"
	"github.com/theinventor/labl-printr/internal/store"
)

type Manager struct {
	store   *store.Store
	virtual *printer.Virtual

	mu     sync.Mutex
	queues map[int64]chan int64
}

func NewManager(s *store.Store, v *printer.Virtual) *Manager {
	return &Manager{store: s, virtual: v, queues: map[int64]chan int64{}}
}

// Resume re-enqueues jobs that were queued or mid-print when the server last
// stopped. Mid-print jobs are marked failed rather than resent — resending
// unverified output is how double labels happen.
func (m *Manager) Resume() {
	pending, err := m.store.QueuedJobs()
	if err != nil {
		log.Printf("jobs: resume scan failed: %v", err)
		return
	}
	for _, j := range pending {
		if j.State == store.JobPrinting {
			m.setState(j.ID, store.JobFailed, "server restarted mid-print; reprint if it didn't come out")
			continue
		}
		m.Enqueue(j)
	}
}

// Enqueue hands a queued job to its printer's worker. Returns false when the
// queue is full — the job is already marked failed and the caller should
// report that, not "queued".
func (m *Manager) Enqueue(j store.Job) bool {
	m.mu.Lock()
	q, ok := m.queues[j.PrinterID]
	if !ok {
		q = make(chan int64, 128)
		m.queues[j.PrinterID] = q
		go m.worker(j.PrinterID, q)
	}
	m.mu.Unlock()
	select {
	case q <- j.ID:
		return true
	default:
		m.setState(j.ID, store.JobFailed, "print queue full")
		return false
	}
}

func (m *Manager) worker(printerID int64, q chan int64) {
	for jobID := range q {
		m.run(printerID, jobID)
	}
}

func (m *Manager) run(printerID, jobID int64) {
	// The render/send path parses stored ZPL; a panic must fail one job, not
	// the whole worker (goroutine panics would kill the server).
	defer func() {
		if r := recover(); r != nil {
			log.Printf("jobs: recovered from panic on job %d: %v", jobID, r)
			m.setState(jobID, store.JobFailed, "internal error while printing")
		}
	}()
	j, err := m.store.Job(jobID)
	if err != nil {
		log.Printf("jobs: load job %d: %v", jobID, err)
		return
	}
	if j.State != store.JobQueued {
		return
	}
	p, err := m.store.Printer(printerID)
	if err != nil {
		m.setState(jobID, store.JobFailed, "printer no longer exists")
		return
	}
	m.setState(jobID, store.JobPrinting, "")

	var sendErr error
	switch p.Kind {
	case store.KindVirtual:
		sendErr = m.virtual.SendJob(jobID, j.ZPL)
	case store.KindBrother:
		sendErr = sendBrother(p, j)
	default:
		t := &printer.TCP{Host: p.Host, Port: p.Port}
		sendErr = t.Send(j.ZPL)
	}
	if sendErr != nil {
		m.setState(jobID, store.JobFailed, sendErr.Error())
		return
	}
	m.setState(jobID, store.JobDone, "")
}

// setState logs persistence failures instead of swallowing them — a job whose
// state can't be written would otherwise resurrect as "queued" after restart
// and print a duplicate.
func (m *Manager) setState(jobID int64, state, errMsg string) {
	if err := m.store.SetJobState(jobID, state, errMsg); err != nil {
		log.Printf("jobs: CRITICAL: failed to mark job %d as %s: %v", jobID, state, err)
	}
}

// sendBrother renders the job's label to a bitmap at the printer's geometry and
// ships it as a Brother raster job. The label is already rendered to exact
// pixels for preview; the Brother just consumes those pixels.
func sendBrother(p store.Printer, j store.Job) error {
	// Render at the Brother's own printable width (696), not the job's stored
	// width — a raw/custom job could carry a wider ^PW that Encode would
	// silently clip.
	width := p.WidthDots
	if width <= 0 {
		width = 696
	}
	png, err := render.PNG(j.ZPL, width, j.LengthDots, p.Dpmm)
	if err != nil {
		return err
	}
	// DK-2251 (black/red 62mm) is the roll loaded on Troy's QL-820NWB; it
	// requires 2-color formatting. TODO: make the media a printer setting once
	// a second roll type is in play.
	// Copies ride one atomic multi-page job so a mid-copy failure can't leave
	// the printer half-done with the job marked failed (→ reprint duplicates).
	data, err := brother.EncodePNG(png, brother.Options{
		Media: brother.DK2251Continuous, AutoCut: true, Copies: j.Copies,
	})
	if err != nil {
		return err
	}
	return brother.Send(p.Host, p.Port, data)
}

// PrinterStatus returns live status for a printer record.
func (m *Manager) PrinterStatus(p store.Printer) printer.Status {
	switch p.Kind {
	case store.KindVirtual:
		return m.virtual.Status()
	case store.KindBrother:
		// No ~HS equivalent over the network; reachability is the only signal.
		if brother.Reachable(p.Host, p.Port) {
			return printer.Status{Ready: true, Reachable: true}
		}
		return printer.Status{Detail: "unreachable"}
	default:
		t := &printer.TCP{Host: p.Host, Port: p.Port}
		return t.Status()
	}
}

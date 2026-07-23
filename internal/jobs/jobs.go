// Package jobs runs the per-printer print queues: one worker goroutine per
// printer, so jobs to the same device serialize while different printers
// print concurrently.
package jobs

import (
	"log"
	"sync"

	"github.com/theinventor/labl-printr/internal/printer"
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

// PrinterStatus returns live status for a printer record.
func (m *Manager) PrinterStatus(p store.Printer) printer.Status {
	if p.Kind == store.KindVirtual {
		return m.virtual.Status()
	}
	t := &printer.TCP{Host: p.Host, Port: p.Port}
	return t.Status()
}

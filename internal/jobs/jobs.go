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
			_ = m.store.SetJobState(j.ID, store.JobFailed, "server restarted mid-print; reprint if it didn't come out")
			continue
		}
		m.Enqueue(j)
	}
}

// Enqueue hands a queued job to its printer's worker.
func (m *Manager) Enqueue(j store.Job) {
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
	default:
		_ = m.store.SetJobState(j.ID, store.JobFailed, "print queue full")
	}
}

func (m *Manager) worker(printerID int64, q chan int64) {
	for jobID := range q {
		m.run(printerID, jobID)
	}
}

func (m *Manager) run(printerID, jobID int64) {
	j, err := m.store.Job(jobID)
	if err != nil || j.State != store.JobQueued {
		return
	}
	p, err := m.store.Printer(printerID)
	if err != nil {
		_ = m.store.SetJobState(jobID, store.JobFailed, "printer no longer exists")
		return
	}
	_ = m.store.SetJobState(jobID, store.JobPrinting, "")

	var sendErr error
	switch p.Kind {
	case "virtual":
		sendErr = m.virtual.SendJob(jobID, j.ZPL)
	default:
		t := &printer.TCP{Host: p.Host, Port: p.Port}
		sendErr = t.Send(j.ZPL)
	}
	if sendErr != nil {
		_ = m.store.SetJobState(jobID, store.JobFailed, sendErr.Error())
		return
	}
	_ = m.store.SetJobState(jobID, store.JobDone, "")
}

// PrinterStatus returns live status for a printer record.
func (m *Manager) PrinterStatus(p store.Printer) printer.Status {
	if p.Kind == "virtual" {
		return m.virtual.Status()
	}
	t := &printer.TCP{Host: p.Host, Port: p.Port}
	return t.Status()
}

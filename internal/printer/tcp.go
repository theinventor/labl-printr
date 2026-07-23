package printer

import (
	"fmt"
	"net"
	"time"
)

// vars, not consts: tests shrink these to drive the drain loop quickly.
var (
	dialTimeout   = 3 * time.Second
	statusTimeout = 2 * time.Second
	drainTimeout  = 45 * time.Second
	drainInterval = 700 * time.Millisecond
)

// TCP is a network Zebra printer speaking raw ZPL on the data port.
type TCP struct {
	Host string
	Port int
}

func (t *TCP) addr() string { return net.JoinHostPort(t.Host, fmt.Sprint(t.Port)) }

// Status queries ~HS over the data port. An unreachable printer or a
// status-read timeout both come back as not ready — Zebra printers in some
// fault states (media out, head open) simply don't answer ~HS.
func (t *TCP) Status() Status {
	conn, err := net.DialTimeout("tcp", t.addr(), dialTimeout)
	if err != nil {
		return Status{Detail: "unreachable: " + err.Error()}
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("~HS")); err != nil {
		return Status{Detail: "write failed: " + err.Error()}
	}
	_ = conn.SetReadDeadline(time.Now().Add(statusTimeout))
	buf := make([]byte, 1024)
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			break
		}
		if st, ok := ParseHS(buf[:total]); ok {
			return st
		}
	}
	if st, ok := ParseHS(buf[:total]); ok {
		return st
	}
	return Status{Reachable: true, Detail: "no ~HS response (printer may be in a fault state)"}
}

// Send delivers ZPL with the honest-status loop: pre-check, write, then poll
// until the receive buffer drains. Port 9100 accepts bytes even when the
// printer can't print, so skipping these checks reports success for labels
// that never existed.
func (t *TCP) Send(zpl string) error {
	if st := t.Status(); !st.Ready {
		if st.Detail == "" {
			st.Detail = "printer not ready"
		}
		return fmt.Errorf("pre-check failed: %s", st.Detail)
	}

	conn, err := net.DialTimeout("tcp", t.addr(), dialTimeout)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	_, werr := conn.Write([]byte(zpl))
	cerr := conn.Close()
	if werr != nil {
		return fmt.Errorf("send: %w", werr)
	}
	if cerr != nil {
		return fmt.Errorf("close: %w", cerr)
	}

	deadline := time.Now().Add(drainTimeout)
	confirmed := false
	for time.Now().Before(deadline) {
		time.Sleep(drainInterval)
		st := t.Status()
		if !st.Responded {
			continue
		}
		confirmed = true
		if st.PaperOut || st.HeadOpen {
			return fmt.Errorf("printer fault while printing: %s", st.Detail)
		}
		if st.Ready && st.FormatsBuffered == 0 {
			return nil
		}
	}
	// Distinguish "printer went quiet" from "printer still busy": both mean
	// the ZPL was delivered, so steer the user to check paper before
	// reprinting rather than implying the job vanished.
	if !confirmed {
		return fmt.Errorf("job was sent but the printer stopped answering status checks — check the printer before reprinting")
	}
	return fmt.Errorf("job was sent but still unconfirmed after %s — check the printer before reprinting", drainTimeout)
}

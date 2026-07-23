package printer

import (
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeZebra listens on loopback and answers ~HS probes from a scripted list;
// each connection either reads a ~HS probe and replies with the next script
// entry, or swallows the payload (a print job).
type fakeZebra struct {
	ln      net.Listener
	scripts chan string
	jobs    atomic.Int32
}

func newFakeZebra(t *testing.T) *fakeZebra {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeZebra{ln: ln, scripts: make(chan string, 32)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeZebra) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	data := string(buf[:n])
	if strings.Contains(data, "~HS") {
		select {
		case s := <-f.scripts:
			if s == "silent" {
				return // no answer, like a faulted printer
			}
			_, _ = conn.Write([]byte(s))
		default:
			return
		}
		return
	}
	// A print job: drain the rest.
	_, _ = io.Copy(io.Discard, conn)
	f.jobs.Add(1)
}

func (f *fakeZebra) tcp() *TCP {
	addr := f.ln.Addr().(*net.TCPAddr)
	return &TCP{Host: "127.0.0.1", Port: addr.Port}
}

func ready() string {
	return "\x02030,0,0,0300,000,0,0,0,000,0,0,0\x03\r\n\x02000,0,0,0,1,2,4,0,00000000,1,000\x03\r\n\x021234,0\x03\r\n"
}

func paperOut() string {
	return "\x02030,1,0,0300,000,0,0,0,000,0,0,0\x03\r\n\x02000,0,0,0,1,2,4,0,00000000,1,000\x03\r\n\x021234,0\x03\r\n"
}

func shrinkTimeouts(t *testing.T) {
	t.Helper()
	oldDrain, oldInterval, oldStatus := drainTimeout, drainInterval, statusTimeout
	drainTimeout, drainInterval, statusTimeout = 900*time.Millisecond, 50*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { drainTimeout, drainInterval, statusTimeout = oldDrain, oldInterval, oldStatus })
}

func TestSendRefusesWhenNotReady(t *testing.T) {
	shrinkTimeouts(t)
	f := newFakeZebra(t)
	f.scripts <- paperOut()
	err := f.tcp().Send("^XA^XZ")
	if err == nil || !strings.Contains(err.Error(), "pre-check failed") {
		t.Fatalf("expected pre-check refusal, got %v", err)
	}
	if f.jobs.Load() != 0 {
		t.Fatal("job bytes were sent despite failed pre-check")
	}
}

func TestSendSucceedsWhenDrained(t *testing.T) {
	shrinkTimeouts(t)
	f := newFakeZebra(t)
	f.scripts <- ready() // pre-check
	f.scripts <- ready() // drain poll: ready, 0 buffered
	if err := f.tcp().Send("^XA^FDhi^FS^XZ"); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if f.jobs.Load() != 1 {
		t.Fatalf("expected 1 job delivered, got %d", f.jobs.Load())
	}
}

func TestSendReportsFaultMidPrint(t *testing.T) {
	shrinkTimeouts(t)
	f := newFakeZebra(t)
	f.scripts <- ready()    // pre-check
	f.scripts <- paperOut() // drain poll: fault
	err := f.tcp().Send("^XA^XZ")
	if err == nil || !strings.Contains(err.Error(), "printer fault while printing") {
		t.Fatalf("expected mid-print fault, got %v", err)
	}
}

func TestSendUnconfirmedWhenPrinterGoesSilent(t *testing.T) {
	shrinkTimeouts(t)
	f := newFakeZebra(t)
	f.scripts <- ready() // pre-check; drain polls get no scripted answers
	err := f.tcp().Send("^XA^XZ")
	if err == nil || !strings.Contains(err.Error(), "stopped answering") {
		t.Fatalf("expected stopped-answering error, got %v", err)
	}
}

func TestStatusUnreachable(t *testing.T) {
	shrinkTimeouts(t)
	dead := &TCP{Host: "127.0.0.1", Port: 1} // nothing listens on port 1
	st := dead.Status()
	if st.Reachable || st.Ready {
		t.Fatalf("expected unreachable, got %+v", st)
	}
}

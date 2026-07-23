package printer

import (
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/theinventor/labl-printr/internal/labels"
	"github.com/theinventor/labl-printr/internal/render"
	"github.com/theinventor/labl-printr/internal/store"
)

// Virtual is the built-in fake Zebra: everything "printed" is rendered to PNG
// and lands in the output tray (virtual_prints). It also runs a real TCP
// listener so anything on the LAN can netcat ZPL at it like a real printer.
type Virtual struct {
	Store *store.Store
}

func (v *Virtual) Status() Status {
	return Status{Ready: true, Reachable: true}
}

// Send renders the payload and files every resulting label in the tray.
func (v *Virtual) Send(zpl string) error {
	return v.print(nil, zpl)
}

// SendJob is Send with job attribution for tray entries.
func (v *Virtual) SendJob(jobID int64, zpl string) error {
	return v.print(&jobID, zpl)
}

// virtualDpmm matches the seeded virtual printer record (203 dpi).
const virtualDpmm = 8

func (v *Virtual) print(jobID *int64, zpl string) error {
	w, l := labels.Dims(zpl)
	imgs, err := render.AllPNG(zpl, w, l, virtualDpmm)
	if err != nil {
		return err
	}
	for _, png := range imgs {
		if err := v.Store.AddVirtualPrint(jobID, zpl, png); err != nil {
			return err
		}
	}
	return nil
}

const (
	maxPayload    = 4 << 20
	maxConcurrent = 8
)

// Listen accepts raw ZPL over TCP like a real printer's port 9100. Each
// connection is one payload (close = end of job), matching how print tools
// actually behave against JetDirect ports.
func (v *Virtual) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	sem := make(chan struct{}, maxConcurrent)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			select {
			case sem <- struct{}{}:
				go func() {
					defer func() { <-sem }()
					v.handle(conn)
				}()
			default:
				_ = conn.Close()
			}
		}
	}()
	return nil
}

func (v *Virtual) handle(conn net.Conn) {
	defer conn.Close()
	// The renderer parses attacker-supplied bytes; a parser panic here must
	// not take down the whole server (HTTP has chi's Recoverer, this doesn't).
	defer func() {
		if r := recover(); r != nil {
			log.Printf("virtual printer: recovered from render panic: %v", r)
		}
	}()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	data, err := io.ReadAll(io.LimitReader(conn, maxPayload))
	if err != nil && len(data) == 0 {
		return
	}
	if len(data) == 0 {
		return
	}
	if len(data) == maxPayload {
		log.Printf("virtual printer: payload hit the %dMB cap — likely truncated, refusing to render a partial label", maxPayload>>20)
		return
	}
	payload := string(data)
	if trimmed := trimToFormat(payload); trimmed != "" {
		if err := v.print(nil, trimmed); err != nil {
			log.Printf("virtual printer: render failed: %v", err)
		}
		return
	}
	if strings.Contains(payload, "~HS") {
		// Answer status probes like a ready printer would.
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, _ = conn.Write([]byte("\x02030,0,0,0300,000,0,0,0,000,0,0,0\x03\r\n\x02000,0,0,0,1,2,4,0,00000000,1,000\x03\r\n\x021234,0\x03\r\n"))
	}
}

func trimToFormat(payload string) string {
	start := strings.Index(payload, "^XA")
	end := strings.LastIndex(payload, "^XZ")
	if start < 0 || end < 0 || end < start {
		return ""
	}
	return payload[start : end+3]
}

// labl-server is the labl-printr network service: web UI, REST API, print
// queues, and the built-in virtual printer, all on one port.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/theinventor/labl-printr/internal/jobs"
	"github.com/theinventor/labl-printr/internal/printer"
	"github.com/theinventor/labl-printr/internal/server"
	"github.com/theinventor/labl-printr/internal/store"
)

func main() {
	var (
		addr        = flag.String("addr", ":5225", "HTTP listen address")
		dataDir     = flag.String("data", "./data", "data directory (SQLite database)")
		virtualAddr = flag.String("virtual-addr", ":9100", "TCP listen address for the virtual printer (empty to disable)")
	)
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	st, err := store.Open(filepath.Join(*dataDir, "labl.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	virtual := &printer.Virtual{Store: st}
	ensureVirtualPrinter(st)

	manager := jobs.NewManager(st, virtual)
	manager.Resume()

	if *virtualAddr != "" {
		if err := virtual.Listen(*virtualAddr); err != nil {
			log.Printf("virtual printer listener disabled: %v", err)
		} else {
			log.Printf("virtual printer listening on %s (send it raw ZPL, it prints PNGs)", *virtualAddr)
		}
	}

	s := &server.Server{Store: st, Jobs: manager, Virtual: virtual}
	log.Printf("labl-printr serving on %s", *addr)
	if err := http.ListenAndServe(*addr, s.Router()); err != nil {
		log.Fatal(err)
	}
}

// ensureVirtualPrinter seeds the built-in virtual printer on first boot so a
// fresh install can print immediately with zero setup.
func ensureVirtualPrinter(st *store.Store) {
	printers, err := st.Printers()
	if err != nil {
		log.Fatalf("list printers: %v", err)
	}
	for _, p := range printers {
		if p.Kind == "virtual" {
			return
		}
	}
	_, err = st.CreatePrinter(store.Printer{
		Name: "Virtual printer", Kind: "virtual", Dpmm: 8, WidthDots: 487,
		IsDefault: len(printers) == 0,
	})
	if err != nil {
		log.Fatalf("seed virtual printer: %v", err)
	}
}

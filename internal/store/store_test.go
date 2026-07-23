package store

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedPrinter(t *testing.T, s *Store) Printer {
	t.Helper()
	p, err := s.CreatePrinter(Printer{Name: "Virtual", Kind: KindVirtual, Dpmm: 8, WidthDots: 487, IsDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCreateJobIdempotency(t *testing.T) {
	s := testStore(t)
	p := seedPrinter(t, s)
	base := Job{PrinterID: p.ID, ZPL: "^XA^XZ", WidthDots: 487, LengthDots: 100, Copies: 1, Source: "test", IdempotencyKey: "key-1"}

	first, existed, err := s.CreateJob(base)
	if err != nil || existed {
		t.Fatalf("first create: existed=%v err=%v", existed, err)
	}
	second, existed, err := s.CreateJob(base)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if !existed || second.ID != first.ID {
		t.Fatalf("idempotent replay returned job %d (existed=%v), want %d", second.ID, existed, first.ID)
	}
}

func TestCreateJobIdempotencyConcurrent(t *testing.T) {
	s := testStore(t)
	p := seedPrinter(t, s)
	const n = 8
	var wg sync.WaitGroup
	ids := make([]int64, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			j, _, err := s.CreateJob(Job{PrinterID: p.ID, ZPL: "^XA^XZ", WidthDots: 487, LengthDots: 100, Copies: 1, Source: "test", IdempotencyKey: "race-key"})
			if err != nil {
				t.Errorf("concurrent create %d: %v", i, err)
				return
			}
			ids[i] = j.ID
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if ids[i] != ids[0] {
			t.Fatalf("concurrent creates diverged: %v", ids)
		}
	}
	jobs, err := s.Jobs(50)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected exactly 1 job row, got %d", len(jobs))
	}
}

func TestJobsListOmitsZPL(t *testing.T) {
	s := testStore(t)
	p := seedPrinter(t, s)
	if _, _, err := s.CreateJob(Job{PrinterID: p.ID, ZPL: "^XA^FDbig^FS^XZ", WidthDots: 487, LengthDots: 100, Copies: 1, Source: "test"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.Jobs(10)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].ZPL != "" {
		t.Fatal("list endpoint should not carry ZPL payloads")
	}
	full, err := s.Job(list[0].ID)
	if err != nil || full.ZPL == "" {
		t.Fatalf("Job(id) must carry ZPL: %q err=%v", full.ZPL, err)
	}
}

func TestVirtualPrintPruning(t *testing.T) {
	s := testStore(t)
	for i := 0; i < trayKeep+25; i++ {
		if err := s.AddVirtualPrint(nil, "^XA^XZ", []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM virtual_prints").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != trayKeep {
		t.Fatalf("tray holds %d, want %d", count, trayKeep)
	}
}

func TestDefaultPrinterFallback(t *testing.T) {
	s := testStore(t)
	a, _ := s.CreatePrinter(Printer{Name: "A", Kind: KindNetwork, Host: "10.0.0.5", Port: 9100, Dpmm: 8, WidthDots: 487})
	if _, err := s.CreatePrinter(Printer{Name: "B", Kind: KindNetwork, Host: "10.0.0.6", Port: 9100, Dpmm: 8, WidthDots: 487}); err != nil {
		t.Fatal(err)
	}
	got, err := s.DefaultPrinter()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != a.ID {
		t.Fatalf("fallback default = %d, want lowest id %d", got.ID, a.ID)
	}
}

func TestLimitClamping(t *testing.T) {
	s := testStore(t)
	p := seedPrinter(t, s)
	for i := 0; i < 3; i++ {
		if _, _, err := s.CreateJob(Job{PrinterID: p.ID, ZPL: "^XA^XZ", WidthDots: 487, LengthDots: 100, Copies: 1, Source: fmt.Sprintf("t%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, limit := range []int{0, -5, 501} {
		if _, err := s.Jobs(limit); err != nil {
			t.Fatalf("Jobs(%d): %v", limit, err)
		}
	}
}

package printer

import (
	"reflect"
	"testing"
)

func TestPrintableRuns(t *testing.T) {
	buf := []byte{0x3a, 0x2c, 0x2e, 'Z', 'D', '4', '2', '1', 0x00, 0x01, 'Z', 'B', 'R', '1', '2', '3', '4', 0xff, 'a', 'b', 0x00}
	got := printableRuns(buf, 4)
	want := []string{":,.ZD421", "ZBR1234"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPrintableRunsDropsShortRuns(t *testing.T) {
	if runs := printableRuns([]byte{'a', 0x00, 'b', 'c', 0x00}, 4); len(runs) != 0 {
		t.Fatalf("expected no runs, got %v", runs)
	}
}

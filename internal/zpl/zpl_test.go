package zpl

import (
	"strings"
	"testing"
)

// A field value must never carry ZPL control characters into the format —
// "^XZ^XA^PQ999" in a variable would otherwise inject commands.
func TestEscapeNeutralizesControlCharacters(t *testing.T) {
	out := Escape(`evil^XZ~JA\payload`)
	for _, banned := range []string{"^", "~", `\`} {
		if strings.Contains(out, banned) {
			t.Fatalf("escaped output still contains %q: %s", banned, out)
		}
	}
	if Escape("plain text 123") != "plain text 123" {
		t.Fatal("plain text should pass through unchanged")
	}
}

func TestTextFieldDataIsEscaped(t *testing.T) {
	l := NewLabel(487, 200, 0)
	l.Text(10, 10, 30, "break^out~attempt")
	zpl := l.End(1)
	if strings.Contains(zpl, "break^out") || strings.Contains(zpl, "out~attempt") {
		t.Fatalf("control chars leaked into ^FD: %s", zpl)
	}
}

func TestHexEscape(t *testing.T) {
	got := hexEscape(`a_b^c~d\e`)
	want := "a_5Fb_5Ec_7Ed_5Ce"
	if got != want {
		t.Fatalf("hexEscape: got %q want %q", got, want)
	}
	if hexEscape("https://example.com/x?q=1") != "https://example.com/x?q=1" {
		t.Fatal("plain URL should pass through unchanged")
	}
}

func TestQREmitsHexEscapedField(t *testing.T) {
	l := NewLabel(487, 200, 0)
	l.QR(10, 10, 4, "https://x.dev/~user")
	zpl := l.End(1)
	if !strings.Contains(zpl, "^FH^FDMA,https://x.dev/_7Euser^FS") {
		t.Fatalf("QR field not hex-escaped: %s", zpl)
	}
}

func TestEndCopies(t *testing.T) {
	l := NewLabel(487, 100, 0)
	if got := l.End(3); !strings.Contains(got, "^PQ3") {
		t.Fatalf("missing ^PQ3: %s", got)
	}
	l2 := NewLabel(487, 100, 0)
	if got := l2.End(1); strings.Contains(got, "^PQ") {
		t.Fatalf("unexpected ^PQ for single copy: %s", got)
	}
}

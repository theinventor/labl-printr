package labels

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theinventor/labl-printr/internal/templates"
)

var sampleVars = map[string]map[string]string{
	"inventory": {
		"name": "M3 socket head screws",
		"sku":  "HW-M3-012",
		"info": "12mm, black oxide, qty 100",
		"url":  "https://inv.ranch.local/items/hw-m3-012",
	},
	"large-print": {"text": "FRAGILE"},
	"small-print": {"text": "HDMI 8ft — office TV, port 2"},
	"packing": {
		"room":     "Kitchen",
		"contents": "Pots and pans\nCutting boards\nKnife block\nMixing bowls",
	},
}

func TestFinalizeTrimsToContent(t *testing.T) {
	outDir := filepath.Join("testdata", "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tpl := range templates.Builtins() {
		t.Run(tpl.ID, func(t *testing.T) {
			est, err := tpl.Render(sampleVars[tpl.ID], templates.DefaultProfile, 1)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Finalize(tpl, sampleVars[tpl.ID], templates.DefaultProfile, 1)
			if err != nil {
				t.Fatal(err)
			}
			if f.LengthDots > est.LengthDots+TrimHeadroom {
				t.Fatalf("trim grew label: est %d → final %d", est.LengthDots, f.LengthDots)
			}
			if !strings.Contains(f.ZPL, fmt.Sprintf("^LL%d", f.LengthDots)) {
				t.Fatalf("final ZPL missing trimmed ^LL%d", f.LengthDots)
			}
			if err := os.WriteFile(filepath.Join(outDir, tpl.ID+".png"), f.PNG, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDims(t *testing.T) {
	w, l := Dims("^XA^PW487^MNN^LL300^XZ")
	if w != 487 || l != 300 {
		t.Fatalf("got %d x %d", w, l)
	}
	w, l = Dims("^XA^FDno geometry^XZ")
	if w != 487 || l != 600 {
		t.Fatalf("fallback got %d x %d", w, l)
	}
}

// A 30-byte payload must not be able to demand a multi-gigabyte render canvas.
func TestDimsClampsHostileGeometry(t *testing.T) {
	w, l := Dims("^XA^PW200000^LL200000^XZ")
	if w != MaxWidthDots || l != MaxLengthDots {
		t.Fatalf("hostile dims not clamped: %d x %d", w, l)
	}
}

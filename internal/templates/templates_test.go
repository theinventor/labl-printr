package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theinventor/labl-printr/internal/render"
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

// TestRenderBuiltins renders each builtin with sample data through the real
// preview engine — catching both ZPL syntax errors and geometry mistakes.
// PNGs land in testdata/out for eyeballing.
func TestRenderBuiltins(t *testing.T) {
	outDir := filepath.Join("testdata", "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tpl := range Builtins() {
		t.Run(tpl.ID, func(t *testing.T) {
			r, err := tpl.Render(sampleVars[tpl.ID], DefaultProfile, 1)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if r.LengthDots < 40 || r.LengthDots > 4000 {
				t.Fatalf("suspicious label length %d dots", r.LengthDots)
			}
			png, err := render.PNG(r.ZPL, DefaultProfile.WidthDots, r.LengthDots, DefaultProfile.Dpmm)
			if err != nil {
				t.Fatalf("preview: %v\nzpl:\n%s", err, r.ZPL)
			}
			if err := os.WriteFile(filepath.Join(outDir, fmt.Sprintf("%s.png", tpl.ID)), png, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// contentBottom-based trimming is exercised via the labels package test; here
// we only guarantee estimates stay generous enough that nothing clips before
// the trim pass runs.

func TestRequiredFields(t *testing.T) {
	tpl, _ := Get("inventory")
	if _, err := tpl.Render(map[string]string{"name": "x"}, DefaultProfile, 1); err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestCustomPlaceholders(t *testing.T) {
	raw := "^XA^FO10,10^A0N,30,0^FD${title}^FS^FO10,50^A0N,20,0^FD${sku}^FS^XZ"
	keys := PlaceholderKeys(raw)
	if len(keys) != 2 || keys[0] != "sku" || keys[1] != "title" {
		t.Fatalf("got keys %v", keys)
	}
	tpl := CustomTemplate("c1", "Custom", raw, 200)
	r, err := tpl.Render(map[string]string{"title": "Hello", "sku": "S-1"}, DefaultProfile, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := "^FDHello^FS"; !strings.Contains(r.ZPL, want) {
		t.Fatalf("substitution failed: %s", r.ZPL)
	}
	if !strings.Contains(r.ZPL, "^PQ2") {
		t.Fatalf("copies not applied: %s", r.ZPL)
	}
}

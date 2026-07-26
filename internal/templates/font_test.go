package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theinventor/labl-printr/internal/render"
)

// TestFontTemplates renders the text templates with handwriting fonts through
// the real preview engine, catching ZPL ^GF encoding and geometry errors.
// PNGs land in testdata/out for eyeballing.
func TestFontTemplates(t *testing.T) {
	out := filepath.Join("testdata", "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tpl, name string
		vars      map[string]string
	}{
		{"packing", "packing-marker", map[string]string{"room": "Garage", "contents": "Impact driver\nBit set\nClamps", "font": "marker"}},
		{"large-print", "large-marker", map[string]string{"text": "FRAGILE", "font": "marker"}},
		{"packing", "packing-patrick", map[string]string{"room": "Kitchen", "contents": "Pots and pans\nCutting boards\nKnife block", "font": "patrick"}},
		{"small-print", "small-kalam", map[string]string{"text": "Christmas ornaments — attic box 3", "font": "kalam"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tpl, _ := Get(c.tpl)
			r, err := tpl.Render(c.vars, DefaultProfile, 1)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if !strings.Contains(r.ZPL, "^GFA,") {
				t.Fatalf("expected a ^GF graphic for a handwriting font: %.80s", r.ZPL)
			}
			png, err := render.PNG(r.ZPL, DefaultProfile.WidthDots, r.LengthDots, 8)
			if err != nil {
				t.Fatalf("preview: %v", err)
			}
			_ = os.WriteFile(filepath.Join(out, c.name+".png"), png, 0o644)
		})
	}
}

// TestSystemFontStaysNative confirms the default face still emits native ZPL
// text (no bitmap), preserving the crisp resident-font look and small payloads.
func TestSystemFontStaysNative(t *testing.T) {
	tpl, _ := Get("large-print")
	r, err := tpl.Render(map[string]string{"text": "FRAGILE", "font": "system"}, DefaultProfile, 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.ZPL, "^GFA,") {
		t.Fatalf("system font should not rasterize: %s", r.ZPL)
	}
	if !strings.Contains(r.ZPL, "^A0N,") {
		t.Fatalf("system font should use native ^A0: %s", r.ZPL)
	}
}

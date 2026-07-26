package fonts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theinventor/labl-printr/internal/render"
)

func TestListHasHandwriting(t *testing.T) {
	list := List()
	if len(list) < 6 {
		t.Fatalf("expected curated faces, got %d", len(list))
	}
	if list[0].ID != "system" {
		t.Fatalf("system face should be first, got %q", list[0].ID)
	}
	var hand int
	for _, f := range list {
		if f.Style == "handwriting" {
			hand++
		}
	}
	if hand < 5 {
		t.Fatalf("expected >=5 handwriting faces, got %d", hand)
	}
}

func TestSystemFaceIsNotBitmap(t *testing.T) {
	if Get("system").IsBitmap() {
		t.Fatal("system face must not render as a bitmap")
	}
	if _, err := Get("system").Render(RenderOptions{Text: "x", SizePx: 20, MaxWidth: 100}); err == nil {
		t.Fatal("system face Render should error")
	}
}

func TestUnknownFaceFallsBack(t *testing.T) {
	if Get("nope").ID != "system" {
		t.Fatal("unknown id should fall back to system")
	}
}

// Render every handwriting face and confirm the ^GF graphic actually renders
// back to a non-blank PNG through zebrash — the real preview/print path.
func TestRenderThroughZebrash(t *testing.T) {
	outDir := filepath.Join("testdata", "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, meta := range List() {
		if meta.Style != "handwriting" {
			continue
		}
		f := Get(meta.ID)
		bm, err := f.Render(RenderOptions{Text: "KITCHEN", SizePx: 90, MaxWidth: 455, Align: AlignCenter, LineGap: 8})
		if err != nil {
			t.Fatalf("%s render: %v", meta.ID, err)
		}
		if bm.Width != 455 || bm.Height < 40 {
			t.Fatalf("%s odd bitmap %dx%d", meta.ID, bm.Width, bm.Height)
		}
		gf := bm.ToZPLGraphic(16, 16)
		if !strings.HasPrefix(gf, "^FO16,16^GFA,") {
			t.Fatalf("%s bad ^GF prefix: %.40s", meta.ID, gf)
		}
		zpl := "^XA^CI28^PW487^MNN^LL" + itoa(bm.Height+32) + "\n" + gf + "^XZ\n"
		png, err := render.PNG(zpl, 487, bm.Height+32, 8)
		if err != nil {
			t.Fatalf("%s zebrash: %v", meta.ID, err)
		}
		if len(png) < 200 {
			t.Fatalf("%s suspiciously tiny PNG", meta.ID)
		}
		if err := os.WriteFile(filepath.Join(outDir, meta.ID+".png"), png, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

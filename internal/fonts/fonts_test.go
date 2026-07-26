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

// A hostile paste must not allocate an unbounded bitmap.
func TestRenderBoundsHostileInput(t *testing.T) {
	f := Get("marker")
	huge := strings.Repeat("line\n", 5000)
	bm, err := f.Render(RenderOptions{Text: huge, SizePx: 40, MaxWidth: 455, Align: AlignLeft})
	if err != nil {
		t.Fatalf("bounded render should succeed by truncating, got %v", err)
	}
	if bm.Height > maxBitmapHeight {
		t.Fatalf("bitmap height %d exceeded bound %d", bm.Height, maxBitmapHeight)
	}
	if _, err := f.Render(RenderOptions{Text: "x", SizePx: 40, MaxWidth: 99999, Align: AlignLeft}); err == nil {
		t.Fatal("expected width-bound rejection")
	}
}

// The reverse graphic must emit ^FR inside the field (after ^FO), the order
// Zebra firmware expects.
func TestReverseGraphicOrder(t *testing.T) {
	f := Get("marker")
	bm, err := f.Render(RenderOptions{Text: "X", SizePx: 40, MaxWidth: 200, Align: AlignCenter})
	if err != nil {
		t.Fatal(err)
	}
	rev := bm.ToZPLGraphicReverse(10, 10)
	if !strings.HasPrefix(rev, "^FO10,10^FR^GFA,") {
		t.Fatalf("reverse graphic has wrong field order: %.30s", rev)
	}
	if strings.Contains(bm.ToZPLGraphic(10, 10), "^FR") {
		t.Fatal("non-reverse graphic should not contain ^FR")
	}
}

// Degenerate text must not panic or produce a zero-dimension bitmap.
func TestRenderDegenerateText(t *testing.T) {
	f := Get("marker")
	for _, txt := range []string{"", "   ", "\n\n\n", "\t"} {
		bm, err := f.Render(RenderOptions{Text: txt, SizePx: 40, MaxWidth: 455, Align: AlignLeft})
		if err != nil {
			t.Fatalf("text %q: %v", txt, err)
		}
		if bm.Width < 1 || bm.Height < 1 {
			t.Fatalf("text %q gave %dx%d", txt, bm.Width, bm.Height)
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

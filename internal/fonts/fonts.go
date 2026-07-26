// Package fonts renders text to 1-bit bitmaps using embedded TTF fonts, so
// labels can use real handwriting/display faces instead of the printer's one
// resident font. The rendered bitmap is what both the ZPL ^GF path and the
// Brother raster path consume — preview and print are the same pixels.
package fonts

import (
	"embed"
	"fmt"
	"image"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed files/*.ttf
var files embed.FS

// Face describes a bundled font available to templates.
type Face struct {
	ID     string `json:"id"`
	Name   string `json:"name"`   // display name
	Style  string `json:"style"`  // "handwriting" | "system"
	file   string // path in the embed FS ("" = built-in ^A0)
	loaded *opentype.Font
}

// registry is the ordered, curated font list. The empty-file "system" face
// maps to ZPL's resident scalable font (^A0) — the original look.
var registry = []*Face{
	{ID: "system", Name: "Printer default", Style: "system", file: ""},
	{ID: "marker", Name: "Marker", Style: "handwriting", file: "files/PermanentMarker-Regular.ttf"},
	{ID: "patrick", Name: "Patrick Hand", Style: "handwriting", file: "files/PatrickHand-Regular.ttf"},
	{ID: "kalam", Name: "Kalam", Style: "handwriting", file: "files/Kalam-Bold.ttf"},
	{ID: "gochi", Name: "Gochi Hand", Style: "handwriting", file: "files/GochiHand-Regular.ttf"},
	{ID: "indie", Name: "Indie Flower", Style: "handwriting", file: "files/IndieFlower-Regular.ttf"},
	{ID: "architect", Name: "Architect", Style: "handwriting", file: "files/ArchitectsDaughter-Regular.ttf"},
}

var (
	mu     sync.Mutex
	byID   = map[string]*Face{}
	inited bool
)

func ensure() error {
	mu.Lock()
	defer mu.Unlock()
	if inited {
		return nil
	}
	for _, f := range registry {
		byID[f.ID] = f
		if f.file == "" {
			continue
		}
		data, err := files.ReadFile(f.file)
		if err != nil {
			return fmt.Errorf("read font %s: %w", f.id(), err)
		}
		ft, err := opentype.Parse(data)
		if err != nil {
			return fmt.Errorf("parse font %s: %w", f.id(), err)
		}
		f.loaded = ft
	}
	inited = true
	return nil
}

func (f *Face) id() string { return f.ID }

// IsBitmap reports whether this face renders to a bitmap (true) or falls back
// to the printer's resident ^A0 font (false).
func (f *Face) IsBitmap() bool { return f.file != "" }

// List returns the curated faces (metadata only).
func List() []Face {
	_ = ensure()
	out := make([]Face, 0, len(registry))
	for _, f := range registry {
		out = append(out, Face{ID: f.ID, Name: f.Name, Style: f.Style})
	}
	return out
}

// Get returns a face by id, falling back to the system face for unknown ids.
func Get(id string) *Face {
	_ = ensure()
	if f, ok := byID[id]; ok {
		return f
	}
	return byID["system"]
}

// TTF returns the raw font file bytes for a bundled face, so the web UI can
// preview each option in its own handwriting. Returns nil for the system face.
func TTF(id string) []byte {
	_ = ensure()
	f, ok := byID[id]
	if !ok || f.file == "" {
		return nil
	}
	data, err := files.ReadFile(f.file)
	if err != nil {
		return nil
	}
	return data
}

// Bitmap is a rendered 1-bit image: Set[y*Width+x] true means a black dot.
type Bitmap struct {
	Width  int
	Height int
	pix    []bool
}

func (b *Bitmap) at(x, y int) bool {
	if x < 0 || y < 0 || x >= b.Width || y >= b.Height {
		return false
	}
	return b.pix[y*b.Width+x]
}

// Line lays out one line of text at the given pixel height and returns its
// advance width in dots, used by callers to size/place the ^GF field. It does
// not draw; use Render for pixels.
type layoutLine struct {
	text  string
	width int
}

// wrap greedily wraps text into lines no wider than maxWidth dots at the given
// point size, mirroring how the built-in ^FB wrapping behaves so estimates and
// output agree.
func (f *Face) wrap(text string, sizePx float64, maxWidth int) ([]layoutLine, error) {
	face, err := opentype.NewFace(f.loaded, &opentype.FaceOptions{Size: sizePx, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer face.Close()

	measure := func(s string) int {
		return font.MeasureString(face, s).Ceil()
	}

	var lines []layoutLine
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, layoutLine{"", 0})
			continue
		}
		cur := words[0]
		for _, w := range words[1:] {
			trial := cur + " " + w
			if measure(trial) > maxWidth {
				lines = append(lines, layoutLine{cur, measure(cur)})
				cur = w
			} else {
				cur = trial
			}
		}
		lines = append(lines, layoutLine{cur, measure(cur)})
	}
	return lines, nil
}

// Align controls horizontal placement within the bitmap width.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// Hard bounds on rasterized text, mirroring the label geometry clamps. These
// stop untrusted text (a giant paste, thousands of newlines, a hostile printer
// widthDots) from forcing a multi-gigabyte image allocation.
const (
	maxBitmapWidth  = 4000
	maxBitmapHeight = 16000
	maxRenderLines  = 200
)

// RenderOptions configures a text render.
type RenderOptions struct {
	Text     string
	SizePx   float64 // cap-ish pixel height driving the font size
	MaxWidth int     // wrap width in dots; also the bitmap width
	MaxLines int     // hard cap on lines drawn (0 = maxRenderLines)
	Align    Align
	LineGap  int // extra dots between wrapped lines
}

// Render lays out and rasterizes text to a 1-bit Bitmap. Returns an error for
// the system face (which has no TTF — callers should use native ZPL text).
func (f *Face) Render(opts RenderOptions) (*Bitmap, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if !f.IsBitmap() {
		return nil, fmt.Errorf("face %q renders via native ZPL, not a bitmap", f.ID)
	}
	if opts.MaxWidth <= 0 {
		return nil, fmt.Errorf("MaxWidth must be positive")
	}
	if opts.MaxWidth > maxBitmapWidth {
		return nil, fmt.Errorf("text width %d exceeds bound %d", opts.MaxWidth, maxBitmapWidth)
	}
	lines, err := f.wrap(opts.Text, opts.SizePx, opts.MaxWidth)
	if err != nil {
		return nil, err
	}
	// Cap line count so a hostile paste can't blow up the image height.
	lineCap := opts.MaxLines
	if lineCap <= 0 || lineCap > maxRenderLines {
		lineCap = maxRenderLines
	}
	if len(lines) > lineCap {
		lines = lines[:lineCap]
	}

	face, err := opentype.NewFace(f.loaded, &opentype.FaceOptions{Size: opts.SizePx, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer face.Close()
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	lineHeight := ascent + descent

	height := len(lines)*lineHeight + (len(lines)-1)*opts.LineGap
	if height < 1 {
		height = 1
	}
	if height > maxBitmapHeight {
		return nil, fmt.Errorf("text height %d exceeds bound %d", height, maxBitmapHeight)
	}
	// Render onto a grayscale image, then threshold to 1-bit. A real
	// grayscale intermediate keeps anti-aliased edges crisp before the
	// hard threshold the thermal head needs.
	img := image.NewGray(image.Rect(0, 0, opts.MaxWidth, height))
	drawer := &font.Drawer{Dst: img, Src: image.White, Face: face}
	// Src white on a zero (black) background would invert; instead draw black
	// glyphs on white by filling white first, then drawing with black source.
	fillWhite(img)
	drawer.Src = image.Black

	y := ascent
	for _, ln := range lines {
		x := 0
		switch opts.Align {
		case AlignCenter:
			x = (opts.MaxWidth - ln.width) / 2
		case AlignRight:
			x = opts.MaxWidth - ln.width
		}
		if x < 0 {
			x = 0
		}
		drawer.Dot = fixed.P(x, y)
		drawer.DrawString(ln.text)
		y += lineHeight + opts.LineGap
	}

	return threshold(img), nil
}

func fillWhite(img *image.Gray) {
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
}

// threshold converts grayscale to 1-bit: any pixel darker than mid-gray
// becomes a black dot.
func threshold(img *image.Gray) *Bitmap {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	b := &Bitmap{Width: w, Height: h, pix: make([]bool, w*h)}
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			if img.GrayAt(xx, yy).Y < 0x80 {
				b.pix[yy*w+xx] = true
			}
		}
	}
	return b
}

// FitAndRender picks the largest size (between minPx and maxPx) at which text
// fits within maxWidth in at most maxLines lines, then renders it. This is the
// bitmap analog of the native ^A0 autosizing the templates already do.
func (f *Face) FitAndRender(text string, maxWidth, maxLines, minPx, maxPx, lineGap int, align Align) (*Bitmap, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if !f.IsBitmap() {
		return nil, fmt.Errorf("face %q renders via native ZPL, not a bitmap", f.ID)
	}
	size := maxPx
	for size > minPx {
		lines, err := f.wrap(text, float64(size), maxWidth)
		if err != nil {
			return nil, err
		}
		// Fit both constraints: line count and the widest line's pixel width.
		// A single long word (e.g. "FRAGILE") never wraps, so width is the
		// binding constraint — without it the glyphs overflow the bitmap.
		widest := 0
		for _, ln := range lines {
			if ln.width > widest {
				widest = ln.width
			}
		}
		if len(lines) <= maxLines && widest <= maxWidth {
			break
		}
		size -= 2
	}
	return f.Render(RenderOptions{Text: text, SizePx: float64(size), MaxWidth: maxWidth, MaxLines: maxLines, Align: align, LineGap: lineGap})
}

// FaceIDs returns the registry ids in order (for tests/validation).
func FaceIDs() []string {
	ids := make([]string, 0, len(registry))
	for _, f := range registry {
		ids = append(ids, f.ID)
	}
	sort.Strings(ids)
	return ids
}

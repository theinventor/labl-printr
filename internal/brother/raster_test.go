package brother

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// makeImage builds a black-on-white test image with an asymmetric mark: a
// black block in the top-left content corner, so a left/right mirror bug is
// unmissable.
func makeImage(w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff // white
	}
	for y := 0; y < h/4; y++ {
		for x := 0; x < w/4; x++ {
			img.SetGray(x, y, color.Gray{0x00}) // black top-left quarter
		}
	}
	return img
}

func TestEncodeHeaderBytes(t *testing.T) {
	img := makeImage(696, 200)
	out, err := Encode(img, Options{Media: DK62Continuous, AutoCut: true})
	if err != nil {
		t.Fatal(err)
	}
	// Preamble: ESC i a, then the invalidate null run.
	if !bytes.HasPrefix(out, []byte{0x1b, 0x69, 0x61, 0x01}) {
		t.Fatal("stream must open with ESC i a")
	}
	for i := 4; i < 4+invalidateNulls; i++ {
		if out[i] != 0 {
			t.Fatalf("byte %d in invalidate run is 0x%02x, want 0x00", i, out[i])
		}
	}
	p := out[4+invalidateNulls:]
	expectPrefix := []byte{
		0x1b, 0x40, // ESC @
		0x1b, 0x69, 0x61, 0x01, // ESC i a raster
		0x1b, 0x69, 0x7a, 0xce, 0x0a, 0x3e, 0x00, 0xc8, 0x00, 0x00, 0x00, 0x00, 0x00, // ESC i z, flags 0xCE, n=200
		0x1b, 0x69, 0x4d, 0x40, // ESC i M autocut
		0x1b, 0x69, 0x41, 0x01, // ESC i A cut every 1
		0x1b, 0x69, 0x4b, 0x08, // ESC i K cut at end (mono)
		0x1b, 0x69, 0x64, 0x23, 0x00, // ESC i d margin 35
		0x4d, 0x00, // M no compression
	}
	if !bytes.HasPrefix(p, expectPrefix) {
		t.Fatalf("header mismatch:\n got %x\nwant %x", p[:len(expectPrefix)], expectPrefix)
	}
	if out[len(out)-1] != 0x1a {
		t.Fatalf("job must end with 0x1A (print+cut), got 0x%02x", out[len(out)-1])
	}
}

func TestEncodeLineCountAndPadding(t *testing.T) {
	// 80 content rows should pad up to the 150-line minimum.
	img := makeImage(696, 80)
	out, err := Encode(img, Options{Media: DK62Continuous})
	if err != nil {
		t.Fatal(err)
	}
	lines := countRasterLines(out)
	if lines != MinLines {
		t.Fatalf("expected padding to %d lines, got %d", MinLines, lines)
	}
	// And the ESC i z count must match the lines actually sent.
	if n := readPrintInfoCount(out); n != MinLines {
		t.Fatalf("ESC i z line count %d != %d lines sent", n, MinLines)
	}
}

func TestEncodeTooTall(t *testing.T) {
	if _, err := Encode(makeImage(696, MaxLines+1), Options{Media: DK62Continuous}); err == nil {
		t.Fatal("expected rejection of an over-tall label")
	}
}

// TestRoundTrip encodes an image, decodes the raster stream back to pixels, and
// confirms the content survives centering + mirroring — proving the label won't
// print backwards or shifted.
func TestRoundTrip(t *testing.T) {
	src := makeImage(696, 300)
	out, err := Encode(src, Options{Media: DK62Continuous})
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeRaster(t, out, DK62Continuous)
	// Compare content pixels.
	mismatch := 0
	for y := 0; y < 300; y++ {
		for x := 0; x < 696; x++ {
			want := src.GrayAt(x, y).Y < 128
			got := decoded[y][x]
			if want != got {
				mismatch++
			}
		}
	}
	if mismatch > 0 {
		t.Fatalf("%d pixels differ after encode/decode round-trip", mismatch)
	}
	// Sanity: the black mark must be top-LEFT in the decoded content, not
	// top-right (which is what a missing mirror would produce).
	if !decoded[10][10] {
		t.Fatal("expected black at content (10,10) — top-left mark lost")
	}
	if decoded[10][690] {
		t.Fatal("black at content (690,10) — image is mirrored left-right")
	}
}

// TestTwoColorFormat verifies the DK-2251 path matches brother_ql: ESC i K
// with the two-color bit, and each line as a black 'w 01' plane plus a blank
// red 'w 02' plane.
func TestTwoColorFormat(t *testing.T) {
	img := makeImage(696, 200)
	out, err := Encode(img, Options{Media: DK2251Continuous, AutoCut: true})
	if err != nil {
		t.Fatal(err)
	}
	if i := bytes.Index(out, []byte{0x1b, 0x69, 0x4b}); i < 0 || out[i+3] != 0x09 {
		t.Fatalf("ESC i K should be 0x09 (cut-at-end + two-color) for DK-2251")
	}
	// First raster line: black plane then red plane.
	bi := bytes.Index(out, []byte{0x77, 0x01, BytesPerRow})
	if bi < 0 {
		t.Fatal("no black 'w 01' plane found")
	}
	red := out[bi+3+BytesPerRow:]
	if !bytes.HasPrefix(red, []byte{0x77, 0x02, BytesPerRow}) {
		t.Fatalf("black plane must be followed by red 'w 02' plane, got %x", red[:3])
	}
	// The red plane must be blank (we only print black).
	redData := red[3 : 3+BytesPerRow]
	for _, b := range redData {
		if b != 0 {
			t.Fatal("red plane should be all zeros")
		}
	}
	if out[len(out)-1] != 0x1a {
		t.Fatal("job must end with 0x1A")
	}
}

// TestCopiesAreOneJob confirms N copies produce N page blocks in a single
// stream: N-1 print commands (0x0C) plus one final print-and-cut (0x1A), with
// the later-page flag set. This keeps copies atomic — no partial-print/reprint
// duplication.
func TestCopiesAreOneJob(t *testing.T) {
	img := makeImage(696, 200)
	out, err := Encode(img, Options{Media: DK2251Continuous, AutoCut: true, Copies: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(out, []byte{0x1b, 0x69, 0x7a}); got != 3 {
		t.Fatalf("expected 3 ESC i z page headers, got %d", got)
	}
	// Exactly one 0x1A (final), and it's the last byte.
	if out[len(out)-1] != 0x1a {
		t.Fatal("stream must end with 0x1A")
	}
	// Later pages carry the 0x01 starting-page flag; page 1 carries 0x00.
	idxs := allIndexes(out, []byte{0x1b, 0x69, 0x7a})
	if out[idxs[0]+11] != 0x00 {
		t.Fatal("first page should have starting-page flag 0x00")
	}
	if out[idxs[1]+11] != 0x01 || out[idxs[2]+11] != 0x01 {
		t.Fatal("later pages should have starting-page flag 0x01")
	}
}

func TestNilImageAndBadMedia(t *testing.T) {
	if _, err := Encode(nil, Options{Media: DK2205Continuous}); err == nil {
		t.Fatal("nil image should error")
	}
	bad := Media{WidthMM: 62, LeftPins: 400, Printable: 400} // 800 > 720 pins
	if _, err := Encode(makeImage(696, 200), Options{Media: bad}); err == nil {
		t.Fatal("media wider than the head should error")
	}
}

func allIndexes(hay, needle []byte) []int {
	var out []int
	for i := 0; i+len(needle) <= len(hay); i++ {
		if bytes.Equal(hay[i:i+len(needle)], needle) {
			out = append(out, i)
		}
	}
	return out
}

// ---- test-only raster decoder ----

func rasterStart(out []byte) int {
	// Skip to the first 'g' (0x67) line command after the header.
	for i := 0; i < len(out)-2; i++ {
		if out[i] == 0x67 && out[i+1] == 0x00 && out[i+2] == BytesPerRow {
			return i
		}
	}
	return -1
}

func countRasterLines(out []byte) int {
	count := 0
	i := rasterStart(out)
	for i >= 0 && i+3+BytesPerRow <= len(out) && out[i] == 0x67 {
		count++
		i += 3 + BytesPerRow
	}
	return count
}

func readPrintInfoCount(out []byte) int {
	// find ESC i z (1B 69 7A) and read n5..n8 little-endian.
	for i := 0; i < len(out)-13; i++ {
		if out[i] == 0x1b && out[i+1] == 0x69 && out[i+2] == 0x7a {
			b := out[i+7 : i+11]
			return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24
		}
	}
	return -1
}

func decodeRaster(t *testing.T, out []byte, m Media) [][]bool {
	t.Helper()
	i := rasterStart(out)
	if i < 0 {
		t.Fatal("no raster lines found")
	}
	var rows [][]bool
	for i+3+BytesPerRow <= len(out) && out[i] == 0x67 {
		data := out[i+3 : i+3+BytesPerRow]
		// Rebuild the logical 720-pin line: transmitted bit k -> pin (719-k).
		line := make([]bool, PinsTotal)
		for k := 0; k < PinsTotal; k++ {
			bit := (data[k/8] >> (7 - uint(k%8))) & 1
			line[PinsTotal-1-k] = bit == 1
		}
		content := make([]bool, m.Printable)
		for x := 0; x < m.Printable; x++ {
			content[x] = line[m.LeftPins+x]
		}
		rows = append(rows, content)
		i += 3 + BytesPerRow
	}
	return rows
}

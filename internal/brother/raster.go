// Package brother speaks the Brother QL raster protocol — the command stream a
// QL-820NWB expects on raw TCP port 9100. labl-printr renders every label to an
// exact 1-bit bitmap (that's how preview works); this package wraps that bitmap
// in the Brother command stream, the way the ZPL packages wrap it for Zebra.
//
// Byte layout is from Brother's "Raster Command Reference (QL-800/810W/820NWB)"
// v1.01, cross-checked against the brother_ql reference implementation.
package brother

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
)

// QL-820NWB hardware constants. The print head is always 720 pins / 90 bytes
// per line regardless of tape width — the tape width only changes how many of
// those pins are blank margin.
const (
	PinsTotal       = 720
	BytesPerRow     = 90
	invalidateNulls = 400 // QL-800 family uses 400, not 200
	MinLines        = 150   // 12.7mm — the printer rejects shorter jobs
	MaxLines        = 11811 // 1000mm

	feedMarginDots = 35 // 3mm leading/trailing feed for continuous tape
)

// Media describes a loaded tape: its width, type byte, and how the 720 head
// pins split into left-margin / printable / right-margin.
type Media struct {
	WidthMM   byte // tape width in mm, e.g. 62
	TypeByte  byte // 0x0A continuous, 0x0B die-cut
	LeftPins  int  // blank pins before the printable area
	Printable int  // printable dots across the tape
	TwoColor  bool // black/red roll (DK-2251) — the printer rejects mono jobs on it
}

// DK2205Continuous is 62mm mono continuous tape (DK-2205): 12 / 696 / 12.
var DK2205Continuous = Media{WidthMM: 62, TypeByte: 0x0A, LeftPins: 12, Printable: 696}

// DK2251Continuous is the 62mm black/red continuous roll. Same geometry, but
// the printer demands a two-color job or it errors "wrong roll type".
var DK2251Continuous = Media{WidthMM: 62, TypeByte: 0x0A, LeftPins: 12, Printable: 696, TwoColor: true}

// DK62Continuous is the historical name for the mono 62mm roll.
var DK62Continuous = DK2205Continuous

// Options configures a raster job.
type Options struct {
	Media     Media
	AutoCut   bool
	Threshold uint8 // luminance below this = a black dot (default 128)
	Copies    int   // number of labels; each is its own page in one job (default 1)
}

// Encode turns a black-on-white image into a complete Brother raster job ready
// to write to the printer's TCP:9100 socket. The image is thresholded to 1-bit,
// left-aligned into the printable area, padded to the full 720-pin line, and
// horizontally mirrored (the head transmits rightmost-dot-first — skip the
// mirror and every label prints backwards).
func Encode(img image.Image, opts Options) ([]byte, error) {
	if opts.Media.Printable == 0 {
		opts.Media = DK62Continuous
	}
	if opts.Threshold == 0 {
		opts.Threshold = 128
	}
	if img == nil {
		return nil, fmt.Errorf("nil image")
	}
	if opts.Media.LeftPins < 0 || opts.Media.Printable <= 0 || opts.Media.LeftPins+opts.Media.Printable > PinsTotal {
		return nil, fmt.Errorf("invalid media geometry: %d+%d must fit in %d pins", opts.Media.LeftPins, opts.Media.Printable, PinsTotal)
	}
	b := img.Bounds()
	content := b.Dy()
	if content <= 0 || b.Dx() <= 0 {
		return nil, fmt.Errorf("empty image")
	}
	// Pad short labels up to the printer's minimum; reject absurdly long ones.
	n := content
	if n < MinLines {
		n = MinLines
	}
	if n > MaxLines {
		return nil, fmt.Errorf("label is %d dots tall, exceeds the %d-dot maximum", n, MaxLines)
	}
	copies := opts.Copies
	if copies < 1 {
		copies = 1
	}

	// Render the raster lines once; copies just repeat the page block so all
	// labels ride a single atomic job (a mid-copy network drop can't leave the
	// printer half-done with the server thinking it failed).
	rasters := renderRasters(img, opts.Media, opts.Threshold, n)

	var buf bytes.Buffer
	writePreamble(&buf)
	for page := 0; page < copies; page++ {
		writePrintInfo(&buf, opts.Media, n, page > 0)
		writeModes(&buf, opts.AutoCut, opts.Media.TwoColor)
		buf.Write(rasters)
		if page == copies-1 {
			buf.WriteByte(0x1a) // last page: print, feed, cut
		} else {
			buf.WriteByte(0x0c) // earlier page: print (cut governed by ESC i A)
		}
	}
	return buf.Bytes(), nil
}

// renderRasters packs the image into n raster-line commands (mono 'g' or the
// two-color 'w' plane pair), padding blank rows past the image height.
func renderRasters(img image.Image, m Media, threshold uint8, n int) []byte {
	b := img.Bounds()
	content := b.Dy()
	line := make([]bool, PinsTotal)
	black := make([]byte, BytesPerRow)
	blank := make([]byte, BytesPerRow)
	var out bytes.Buffer
	for y := 0; y < n; y++ {
		for i := range line {
			line[i] = false
		}
		if y < content {
			py := b.Min.Y + y
			for x := 0; x < m.Printable; x++ {
				px := b.Min.X + x
				if px >= b.Max.X {
					break
				}
				if luminance(img.At(px, py)) < threshold {
					line[m.LeftPins+x] = true
				}
			}
		}
		packMirrored(black, line)
		if m.TwoColor {
			out.Write([]byte{0x77, 0x01, BytesPerRow}) // 'w' black plane
			out.Write(black)
			out.Write([]byte{0x77, 0x02, BytesPerRow}) // 'w' red plane (empty)
			out.Write(blank)
		} else {
			out.Write([]byte{0x67, 0x00, BytesPerRow}) // 'g' mono
			out.Write(black)
		}
	}
	return out.Bytes()
}

// writePreamble matches the brother_ql reference ordering exactly: enter raster
// mode, clear the command buffer, initialize, re-enter raster mode.
func writePreamble(buf *bytes.Buffer) {
	buf.Write([]byte{0x1b, 0x69, 0x61, 0x01}) // ESC i a  raster mode
	buf.Write(make([]byte, invalidateNulls))  // invalidate / clear buffer
	buf.Write([]byte{0x1b, 0x40})             // ESC @  initialize
	buf.Write([]byte{0x1b, 0x69, 0x61, 0x01}) // ESC i a  raster mode
}

// writePrintInfo emits ESC i z: which fields are valid, media type/width,
// length (0 for continuous), and the raster line count as little-endian uint32.
// Flags 0xCE matches the brother_ql reference (type+width+length valid, print
// quality, recovery on).
func writePrintInfo(buf *bytes.Buffer, m Media, n int, laterPage bool) {
	const flags = 0xCE
	buf.Write([]byte{0x1b, 0x69, 0x7a, flags, m.TypeByte, m.WidthMM, 0x00})
	buf.Write([]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)})
	page := byte(0x00) // starting page
	if laterPage {
		page = 0x01
	}
	buf.Write([]byte{page, 0x00})
}

func writeModes(buf *bytes.Buffer, autoCut, twoColor bool) {
	var cut byte
	if autoCut {
		cut = 0x40 // ESC i M bit 6 = auto cut
	}
	expanded := byte(0x08) // ESC i K bit 3 = cut at end
	if twoColor {
		expanded |= 0x01 // bit 0 = two-color (required by DK-2251)
	}
	buf.Write([]byte{0x1b, 0x69, 0x4d, cut})                 // ESC i M  various mode
	buf.Write([]byte{0x1b, 0x69, 0x41, 0x01})                // ESC i A  cut every 1 label
	buf.Write([]byte{0x1b, 0x69, 0x4b, expanded})            // ESC i K  expanded mode
	buf.Write([]byte{0x1b, 0x69, 0x64, feedMarginDots, 0x00}) // ESC i d  feed margin
	buf.Write([]byte{0x4d, 0x00})                            // M  no compression
}

// packMirrored packs a 720-element logical line (index 0 = leftmost dot) into
// 90 bytes with the head's orientation: transmitted bit i corresponds to pin
// (719-i), MSB first. Equivalent to reversing the line then packing normally.
func packMirrored(row []byte, line []bool) {
	for j := range row {
		row[j] = 0
	}
	for i := 0; i < PinsTotal; i++ {
		if line[PinsTotal-1-i] {
			row[i/8] |= 1 << (7 - uint(i%8))
		}
	}
}

func luminance(c color.Color) uint8 {
	r, g, b, a := c.RGBA()
	// Transparent pixels (premultiplied 0,0,0,0) would otherwise read as black
	// and print a solid background — treat anything mostly-transparent as white.
	if a < 0x8000 {
		return 0xff
	}
	// Rec. 601 luma; RGBA() returns 16-bit, shift back to 8-bit.
	y := (299*r + 587*g + 114*b) / 1000
	return uint8(y >> 8)
}

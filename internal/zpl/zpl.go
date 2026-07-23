// Package zpl is a minimal ZPL II command builder — enough surface for
// labl-printr's templates, kept in-house so the project stays MIT-licensed
// (the full-featured JS/Python ZPL layout engines are GPL/AGPL).
package zpl

import (
	"fmt"
	"strings"
)

// Justification values for text blocks (^FB).
const (
	JustifyLeft   = "L"
	JustifyCenter = "C"
	JustifyRight  = "R"
)

// Label accumulates ZPL commands for a single ^XA…^XZ format.
type Label struct {
	buf        strings.Builder
	widthDots  int
	lengthDots int
	leftShift  int
}

// NewLabel starts a format for a label widthDots wide and lengthDots long.
// leftShift is applied via ^LS for printers that center narrow media under
// a wider printhead (ZD-series quirk).
func NewLabel(widthDots, lengthDots, leftShift int) *Label {
	l := &Label{widthDots: widthDots, lengthDots: lengthDots, leftShift: leftShift}
	l.buf.WriteString("^XA\n")
	l.buf.WriteString("^CI28\n") // UTF-8
	fmt.Fprintf(&l.buf, "^PW%d\n", widthDots)
	fmt.Fprintf(&l.buf, "^MNN^LL%d\n", lengthDots)
	fmt.Fprintf(&l.buf, "^LH0,0^LS%d\n", leftShift)
	return l
}

// escape sanitizes field data: ^ and ~ are ZPL control characters, and \ is
// the hex-escape introducer inside ^FD.
func escape(s string) string {
	r := strings.NewReplacer("^", " ", "~", " ", "\\", "/")
	return r.Replace(s)
}

// Text places a single line of scalable font A0 text at x,y with the given
// character height in dots.
func (l *Label) Text(x, y, fontHeight int, text string) {
	fmt.Fprintf(&l.buf, "^FO%d,%d^A0N,%d,%d^FD%s^FS\n", x, y, fontHeight, 0, escape(text))
}

// TextBlock places wrapped text (^FB) in a column widthDots wide, up to
// maxLines lines with lineGap extra dots between lines.
func (l *Label) TextBlock(x, y, fontHeight, width, maxLines, lineGap int, justify, text string) {
	fmt.Fprintf(&l.buf, "^FO%d,%d^A0N,%d,0^FB%d,%d,%d,%s,0^FD%s^FS\n",
		x, y, fontHeight, width, maxLines, lineGap, justify, escape(text))
}

// InverseBar draws a filled bar and switches the fields drawn inside fn to
// white-on-black (^FR reverses each field against the bar).
func (l *Label) InverseText(x, y, w, h, fontHeight int, text string) {
	fmt.Fprintf(&l.buf, "^FO%d,%d^GB%d,%d,%d,B,0^FS\n", x, y, w, h, h)
	textY := y + (h-fontHeight)/2
	fmt.Fprintf(&l.buf, "^FO%d,%d^A0N,%d,0^FB%d,1,0,C,0^FR^FD%s^FS\n",
		x, textY, fontHeight, w, escape(text))
}

// Box draws a rectangle outline.
func (l *Label) Box(x, y, w, h, thickness int) {
	fmt.Fprintf(&l.buf, "^FO%d,%d^GB%d,%d,%d^FS\n", x, y, w, h, thickness)
}

// HLine draws a horizontal rule.
func (l *Label) HLine(x, y, w, thickness int) {
	fmt.Fprintf(&l.buf, "^FO%d,%d^GB%d,%d,%d^FS\n", x, y, w, thickness, thickness)
}

// QR places a model-2 QR code. magnification 1–10; data is encoded in
// automatic mode with ECC level M (good default for URLs).
func (l *Label) QR(x, y, magnification int, data string) {
	fmt.Fprintf(&l.buf, "^FO%d,%d^BQN,2,%d^FDMA,%s^FS\n", x, y, magnification, escape(data))
}

// Raw appends a raw ZPL fragment verbatim.
func (l *Label) Raw(fragment string) {
	l.buf.WriteString(fragment)
	if !strings.HasSuffix(fragment, "\n") {
		l.buf.WriteString("\n")
	}
}

// End closes the format. copies > 1 adds ^PQ.
func (l *Label) End(copies int) string {
	if copies > 1 {
		fmt.Fprintf(&l.buf, "^PQ%d\n", copies)
	}
	l.buf.WriteString("^XZ\n")
	return l.buf.String()
}

// EstCharWidth conservatively estimates the average advance width in dots of
// font A0 at the given height. Used for autosizing/wrap estimates; the preview
// renderer is the source of truth.
func EstCharWidth(fontHeight int) float64 {
	return float64(fontHeight) * 0.52
}

// EstLines estimates how many lines text occupies when wrapped into width
// dots at fontHeight, mirroring ^FB's greedy word wrap.
func EstLines(text string, fontHeight, width int) int {
	charW := EstCharWidth(fontHeight)
	maxChars := int(float64(width) / charW)
	if maxChars < 1 {
		maxChars = 1
	}
	lines := 0
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines++
			continue
		}
		cur := 0
		lines++
		for _, w := range words {
			need := len([]rune(w))
			if cur > 0 {
				need++ // leading space
			}
			if cur+need > maxChars && cur > 0 {
				lines++
				cur = len([]rune(w))
			} else {
				cur += need
			}
		}
	}
	if lines < 1 {
		lines = 1
	}
	return lines
}

// FitFontHeight returns the largest font height ≤ max (and ≥ min) at which the
// longest word of text still fits in width dots.
func FitFontHeight(text string, width, min, max int) int {
	longest := 1
	for _, w := range strings.Fields(text) {
		if n := len([]rune(w)); n > longest {
			longest = n
		}
	}
	h := max
	for h > min {
		if float64(longest)*EstCharWidth(h) <= float64(width) {
			break
		}
		h -= 2
	}
	if h < min {
		h = min
	}
	return h
}

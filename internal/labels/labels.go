// Package labels is the finishing pipeline: template + variables → final ZPL
// with an exact ^LL, plus the preview PNG of those exact bytes.
package labels

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"regexp"
	"strconv"

	"github.com/theinventor/labl-printr/internal/render"
	"github.com/theinventor/labl-printr/internal/templates"
)

const (
	bottomMargin = 16
	minLength    = 64
)

var llRe = regexp.MustCompile(`\^LL(\d+)`)

// Final is a fully finished, printable label.
type Final struct {
	ZPL        string
	WidthDots  int
	LengthDots int
	PNG        []byte
}

// Finalize renders a template, measures where the ink actually ends, and
// trims the label length to fit. Custom (designer) templates keep their
// authored canvas length — the designer user chose it deliberately.
func Finalize(t *templates.Template, vars map[string]string, p templates.Profile, copies int) (Final, error) {
	if p.WidthDots <= 0 {
		p = templates.DefaultProfile
	}
	r, err := t.Render(vars, p, copies)
	if err != nil {
		return Final{}, err
	}
	if !t.Builtin {
		img, err := render.PNG(r.ZPL, p.WidthDots, r.LengthDots, p.Dpmm)
		if err != nil {
			return Final{}, err
		}
		return Final{ZPL: r.ZPL, WidthDots: p.WidthDots, LengthDots: r.LengthDots, PNG: img}, nil
	}

	// Render with headroom so nothing clips, then measure true content height.
	generous := r.LengthDots + 80
	raw, err := render.PNG(withLength(r.ZPL, generous), p.WidthDots, generous, p.Dpmm)
	if err != nil {
		return Final{}, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return Final{}, fmt.Errorf("decode preview: %w", err)
	}
	bottom := contentBottom(img)
	length := bottom + bottomMargin
	if length < minLength {
		length = minLength
	}

	finalZPL := withLength(r.ZPL, length)
	finalPNG, err := render.PNG(finalZPL, p.WidthDots, length, p.Dpmm)
	if err != nil {
		return Final{}, err
	}
	return Final{ZPL: finalZPL, WidthDots: p.WidthDots, LengthDots: length, PNG: finalPNG}, nil
}

func withLength(zpl string, dots int) string {
	return llRe.ReplaceAllString(zpl, "^LL"+strconv.Itoa(dots))
}

// contentBottom returns the y of the lowest dark pixel, scanning upward.
// The rendered scale may exceed 1px per dot only if zebrash changes behavior;
// today 1 dot = 1 px at the requested dpmm, so y maps directly to dots.
func contentBottom(img image.Image) int {
	b := img.Bounds()
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, a := img.At(x, y).RGBA()
			if a > 0x7fff && (r+g+bb)/3 < 0x7fff {
				return y - b.Min.Y + 1
			}
		}
	}
	return minLength
}

// Dims extracts ^PW and ^LL from raw ZPL (for payloads that arrive from the
// outside, e.g. the virtual printer's socket). Falls back to the default
// profile's geometry.
func Dims(zpl string) (widthDots, lengthDots int) {
	widthDots = templates.DefaultProfile.WidthDots
	lengthDots = 600
	if m := regexp.MustCompile(`\^PW(\d+)`).FindStringSubmatch(zpl); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
			widthDots = v
		}
	}
	if m := llRe.FindStringSubmatch(zpl); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
			lengthDots = v
		}
	}
	return widthDots, lengthDots
}

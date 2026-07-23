// Package render turns ZPL into PNG previews via the embedded zebrash engine —
// the same pixels the web UI shows and the virtual printer "prints".
package render

import (
	"bytes"
	"fmt"

	"github.com/ingridhq/zebrash"
	"github.com/ingridhq/zebrash/drawers"
)

// PNG renders the first label of a ZPL format to PNG at the given physical
// geometry. widthDots/lengthDots are in printer dots at dpmm resolution.
func PNG(zpl string, widthDots, lengthDots, dpmm int) ([]byte, error) {
	imgs, err := AllPNG(zpl, widthDots, lengthDots, dpmm)
	if err != nil {
		return nil, err
	}
	return imgs[0], nil
}

// AllPNG renders every label in a ZPL payload (a job may contain several
// ^XA…^XZ formats) to PNGs.
func AllPNG(zpl string, widthDots, lengthDots, dpmm int) ([][]byte, error) {
	if dpmm <= 0 {
		dpmm = 8
	}
	// Last line of defense against giant-canvas allocations — geometry should
	// already be clamped upstream (labels.Dims, designer import validation).
	if widthDots > 4000 || lengthDots > 16000 {
		return nil, fmt.Errorf("label dimensions %dx%d dots exceed hardware-plausible bounds", widthDots, lengthDots)
	}
	labels, err := zebrash.NewParser().Parse([]byte(zpl))
	if err != nil {
		return nil, fmt.Errorf("parse zpl: %w", err)
	}
	if len(labels) == 0 {
		return nil, fmt.Errorf("no printable labels in payload")
	}
	opts := drawers.DrawerOptions{
		LabelWidthMm:  float64(widthDots) / float64(dpmm),
		LabelHeightMm: float64(lengthDots) / float64(dpmm),
		Dpmm:          dpmm,
	}
	drawer := zebrash.NewDrawer()
	out := make([][]byte, 0, len(labels))
	for _, lbl := range labels {
		var buf bytes.Buffer
		if err := drawer.DrawLabelAsPng(lbl, &buf, opts); err != nil {
			return nil, fmt.Errorf("render label: %w", err)
		}
		out = append(out, buf.Bytes())
	}
	return out, nil
}

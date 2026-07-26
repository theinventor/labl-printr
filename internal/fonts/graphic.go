package fonts

import (
	"fmt"
	"strings"
)

// ToZPLGraphic encodes a Bitmap as a ^GFA hex graphic field placed at x,y.
// ^GFA takes: compression 'A' (ASCII hex), total bytes, total bytes, bytes
// per row, then the hex bitmap data. Bit 1 = black dot. Rows are padded to a
// whole number of bytes, MSB first — the standard ZPL graphic layout that
// zebrash and real Zebra printers both render.
func (b *Bitmap) ToZPLGraphic(x, y int) string {
	bytesPerRow := (b.Width + 7) / 8
	total := bytesPerRow * b.Height

	var sb strings.Builder
	fmt.Fprintf(&sb, "^FO%d,%d^GFA,%d,%d,%d,", x, y, total, total, bytesPerRow)
	const hexDigits = "0123456789ABCDEF"
	for yy := 0; yy < b.Height; yy++ {
		for bx := 0; bx < bytesPerRow; bx++ {
			var v byte
			for bit := 0; bit < 8; bit++ {
				px := bx*8 + bit
				if b.at(px, yy) {
					v |= 1 << (7 - bit)
				}
			}
			sb.WriteByte(hexDigits[v>>4])
			sb.WriteByte(hexDigits[v&0x0f])
		}
	}
	sb.WriteString("^FS\n")
	return sb.String()
}

// Rows returns the bitmap as packed big-endian bytes per row — the form the
// Brother raster transport will consume directly (no ZPL wrapper).
func (b *Bitmap) Rows() (bytesPerRow int, rows [][]byte) {
	bytesPerRow = (b.Width + 7) / 8
	rows = make([][]byte, b.Height)
	for yy := 0; yy < b.Height; yy++ {
		row := make([]byte, bytesPerRow)
		for bx := 0; bx < bytesPerRow; bx++ {
			var v byte
			for bit := 0; bit < 8; bit++ {
				if b.at(bx*8+bit, yy) {
					v |= 1 << (7 - bit)
				}
			}
			row[bx] = v
		}
		rows[yy] = row
	}
	return bytesPerRow, rows
}

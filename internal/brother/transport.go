package brother

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"net"
	"time"
)

// EncodePNG decodes a PNG label (the exact bytes labl-printr renders for
// preview) and encodes it as a Brother raster job.
func EncodePNG(pngData []byte, opts Options) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("decode label png: %w", err)
	}
	return Encode(img, opts)
}

// Send writes a raster job to the printer's raw port and closes. Port 9100 is a
// no-ack sink — a successful write means the printer accepted the bytes, the
// same fire-and-forget contract the Zebra path uses when status isn't readable.
func Send(host string, port int, data []byte) error {
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect to printer: %w", err)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	n, werr := conn.Write(data)
	cerr := conn.Close()
	if werr != nil {
		return fmt.Errorf("send to printer: %w", werr)
	}
	if n != len(data) {
		return fmt.Errorf("short write: sent %d of %d bytes", n, len(data))
	}
	if cerr != nil {
		return fmt.Errorf("close connection: %w", cerr)
	}
	return nil
}

// Reachable reports whether the printer accepts a TCP connection on its raw
// port — the only health signal available when status readback is off.
func Reachable(host string, port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

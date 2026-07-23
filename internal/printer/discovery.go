package printer

import (
	"net"
	"strings"
	"time"
	"unicode"
)

// Discovered is one printer that answered the Zebra discovery broadcast.
type Discovered struct {
	IP   string   `json:"ip"`
	Info []string `json:"info"` // printable fields from the response, e.g. model, name, serial
}

// Zebra's proprietary discovery: broadcast magic bytes to UDP 4201; printers
// unicast back a packet starting 0x3a2c2e containing identity + network info.
// Protocol per https://jfr.im/blog/2024/09/zebra-network-discovery-protocol/
// — field offsets unverified against real hardware, so responses are surfaced
// as printable strings plus source IP until the ZD421 is online to test.
var discoveryProbe = []byte{0x2e, 0x2c, 0x3a, 0x01, 0x00, 0x00}

// Discover broadcasts and collects responses for the given window.
func Discover(wait time.Duration) ([]Discovered, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	bcast := &net.UDPAddr{IP: net.IPv4bcast, Port: 4201}
	if _, err := conn.WriteTo(discoveryProbe, bcast); err != nil {
		return nil, err
	}

	_ = conn.SetReadDeadline(time.Now().Add(wait))
	seen := map[string]bool{}
	var found []Discovered
	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			break // deadline reached
		}
		if n < 3 || buf[0] != 0x3a || buf[1] != 0x2c || buf[2] != 0x2e {
			continue
		}
		ip := strings.Split(addr.String(), ":")[0]
		if seen[ip] {
			continue
		}
		seen[ip] = true
		found = append(found, Discovered{IP: ip, Info: printableRuns(buf[:n], 4)})
	}
	return found, nil
}

// printableRuns extracts ASCII runs of at least min chars — enough to show
// model / device name / serial without knowing exact packet offsets.
func printableRuns(b []byte, min int) []string {
	var runs []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= min {
			runs = append(runs, strings.TrimSpace(cur.String()))
		}
		cur.Reset()
	}
	for _, c := range b {
		if c >= 0x20 && c < 0x7f && unicode.IsPrint(rune(c)) {
			cur.WriteByte(c)
		} else {
			flush()
		}
	}
	flush()
	return runs
}

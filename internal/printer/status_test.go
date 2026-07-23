package printer

import (
	"strings"
	"testing"
)

// hsResponse builds an STX/ETX-wrapped ~HS triple like real hardware sends.
func hsResponse(s1, s2, s3 string) []byte {
	return []byte("\x02" + s1 + "\x03\r\n\x02" + s2 + "\x03\r\n\x02" + s3 + "\x03\r\n")
}

const (
	readyS1 = "030,0,0,0300,000,0,0,0,000,0,0,0"
	readyS2 = "000,0,0,0,1,2,4,0,00000000,1,000"
	readyS3 = "1234,0"
)

func TestParseHSReady(t *testing.T) {
	st, ok := ParseHS(hsResponse(readyS1, readyS2, readyS3))
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if !st.Ready || st.PaperOut || st.Paused || st.HeadOpen || st.FormatsBuffered != 0 {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestParseHSFaults(t *testing.T) {
	cases := []struct {
		name   string
		s1, s2 string
		check  func(Status) bool
	}{
		{"paper out", "030,1,0,0300,000,0,0,0,000,0,0,0", readyS2, func(s Status) bool { return s.PaperOut && !s.Ready && strings.Contains(s.Detail, "media out") }},
		{"paused", "030,0,1,0300,000,0,0,0,000,0,0,0", readyS2, func(s Status) bool { return s.Paused && !s.Ready && strings.Contains(s.Detail, "paused") }},
		{"head open", readyS1, "000,0,1,0,1,2,4,0,00000000,1,000", func(s Status) bool { return s.HeadOpen && !s.Ready && strings.Contains(s.Detail, "head open") }},
		{"formats buffered", "030,0,0,0300,002,0,0,0,000,0,0,0", readyS2, func(s Status) bool { return s.FormatsBuffered == 2 && s.Ready }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, ok := ParseHS(hsResponse(c.s1, c.s2, readyS3))
			if !ok {
				t.Fatal("expected parse to succeed")
			}
			if !c.check(st) {
				t.Fatalf("unexpected status: %+v", st)
			}
		})
	}
}

func TestParseHSGarbage(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte(""), []byte("hello"), hsResponse("1,2", "3,4", "5")} {
		if _, ok := ParseHS(raw); ok {
			t.Fatalf("expected parse failure for %q", raw)
		}
	}
}

// The virtual printer's canned ~HS answer must decode as a ready printer, or
// status checks against it would lie.
func TestVirtualCannedHSDecodesReady(t *testing.T) {
	canned := []byte("\x02030,0,0,0300,000,0,0,0,000,0,0,0\x03\r\n\x02000,0,0,0,1,2,4,0,00000000,1,000\x03\r\n\x021234,0\x03\r\n")
	st, ok := ParseHS(canned)
	if !ok || !st.Ready {
		t.Fatalf("virtual canned response not ready: ok=%v %+v", ok, st)
	}
}

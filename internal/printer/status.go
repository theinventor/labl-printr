// Package printer talks to physical and virtual label printers: raw-9100
// transport, ~HS status, UDP discovery, and the built-in virtual printer.
package printer

import (
	"strconv"
	"strings"
)

// Status is the decoded subset of Zebra host status that decides whether a
// job should be sent and whether it finished.
type Status struct {
	Ready           bool   `json:"ready"`
	Reachable       bool   `json:"reachable"`
	PaperOut        bool   `json:"paperOut"`
	Paused          bool   `json:"paused"`
	HeadOpen        bool   `json:"headOpen"`
	FormatsBuffered int    `json:"formatsBuffered"`
	Detail          string `json:"detail,omitempty"`
	// Responded means an actual ~HS answer was parsed. A printer that accepts
	// TCP but stays silent on ~HS is in a fault state per Zebra's docs — the
	// flag fields are meaningless unless this is true.
	Responded bool `json:"-"`
}

// ParseHS decodes the three ~HS response strings. The printer wraps each in
// STX (0x02) … ETX (0x03). Reference: ZPL II Programming Guide, ~HS.
func ParseHS(raw []byte) (Status, bool) {
	cleaned := strings.FieldsFunc(string(raw), func(r rune) bool {
		return r == '\x02' || r == '\x03' || r == '\r' || r == '\n'
	})
	var lines []string
	for _, l := range cleaned {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	if len(lines) < 2 {
		return Status{}, false
	}
	s1 := strings.Split(lines[0], ",")
	s2 := strings.Split(lines[1], ",")
	if len(s1) < 12 || len(s2) < 8 {
		return Status{}, false
	}
	st := Status{Reachable: true, Responded: true}
	st.PaperOut = s1[1] == "1"
	st.Paused = s1[2] == "1"
	st.FormatsBuffered, _ = strconv.Atoi(s1[4])
	st.HeadOpen = s2[2] == "1"
	st.Ready = !st.PaperOut && !st.Paused && !st.HeadOpen

	var problems []string
	if st.PaperOut {
		problems = append(problems, "media out")
	}
	if st.Paused {
		problems = append(problems, "paused")
	}
	if st.HeadOpen {
		problems = append(problems, "head open")
	}
	st.Detail = strings.Join(problems, ", ")
	return st, true
}

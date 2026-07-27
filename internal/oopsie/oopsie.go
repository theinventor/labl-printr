// Package oopsie reports exceptions to Troy's self-hosted Oopsie tracker. It's
// the single ingest point: the Go server's panics and job failures, plus
// browser errors forwarded from the web UI, all flow through here. Reporting is
// fire-and-forget — it never blocks or breaks the thing that failed.
package oopsie

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	endpoint = "https://oopsie.sunflower-vacations.com/api/v1/exceptions"
	// Project-scoped ingest key for the "labl-printr" Oopsie project. This is a
	// report-only key (the Sentry-DSN model — it can create exceptions, not read
	// or administer), so it's safe to ship in the binary.
	projectKey  = "be8de9b1bc161e82ba6ffb373669a6bef283620a410f6afb7b8ae271afdd8053"
	environment = "production"
)

var (
	client   = &http.Client{Timeout: 5 * time.Second}
	hostname string
)

func init() {
	hostname, _ = os.Hostname()
}

// FirstLine locates a fault in source.
type FirstLine struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Method string `json:"method"`
}

// Report sends one exception. class is a short error type ("panic",
// "job_failed", "client_error"), message is the human text, backtrace is
// optional stack frames, ctx is arbitrary debugging context. Returns
// immediately; the HTTP call runs in the background.
func Report(class, message string, backtrace []string, ctx map[string]any) {
	go send(class, message, backtrace, ctx, false)
}

func send(class, message string, backtrace []string, ctx map[string]any, handled bool) {
	body := map[string]any{
		"notifier":  "labl-printr",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"app": map[string]string{
			"name":        "labl-printr",
			"environment": environment,
		},
		"error": map[string]any{
			"class_name": class,
			"message":    message,
			"backtrace":  backtrace,
			"first_line": firstLine(backtrace),
			"causes":     []any{},
			"handled":    handled,
		},
		"context": ctx,
		"server": map[string]any{
			"hostname": hostname,
			"pid":      os.Getpid(),
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+projectKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// firstLine parses the top backtrace frame "file:line:method" into structure.
func firstLine(backtrace []string) FirstLine {
	if len(backtrace) == 0 {
		return FirstLine{}
	}
	top := backtrace[0]
	fl := FirstLine{File: top}
	parts := strings.SplitN(top, ":", 3)
	if len(parts) >= 2 {
		fl.File = parts[0]
		if n := atoi(parts[1]); n > 0 {
			fl.Line = n
		}
		if len(parts) == 3 {
			fl.Method = strings.TrimPrefix(strings.TrimSpace(parts[2]), "in ")
		}
	}
	return fl
}

func atoi(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// config lives at ~/.config/labl/config.json — no env vars, one obvious file.
type config struct {
	Server string `json:"server"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "labl", "config.json")
}

func loadConfig() config {
	c := config{Server: "http://localhost:5225"}
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func saveConfig(c config) error {
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(configPath(), data, 0o644)
}

type client struct {
	base string
	http *http.Client
}

func newClient() *client {
	return &client{base: loadConfig().Server, http: &http.Client{Timeout: 90 * time.Second}}
}

func (c *client) do(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach labl-printr at %s (%v) — is the server running? set it with: labl config set-server http://host:5225", c.base, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// API shapes (mirrors of the server's JSON).

type templateInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`
	Fields      []struct {
		Key         string `json:"key"`
		Label       string `json:"label"`
		Type        string `json:"type"`
		Required    bool   `json:"required"`
		Placeholder string `json:"placeholder"`
	} `json:"fields"`
}

type printerInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Dpmm      int    `json:"dpmm"`
	WidthDots int    `json:"widthDots"`
	IsDefault bool   `json:"isDefault"`
}

type jobInfo struct {
	ID          int64  `json:"id"`
	PrinterName string `json:"printerName"`
	TemplateID  string `json:"templateId"`
	State       string `json:"state"`
	Error       string `json:"error"`
	Copies      int    `json:"copies"`
	Source      string `json:"source"`
	CreatedAt   string `json:"createdAt"`
}

type statusInfo struct {
	Ready           bool   `json:"ready"`
	Reachable       bool   `json:"reachable"`
	PaperOut        bool   `json:"paperOut"`
	Paused          bool   `json:"paused"`
	HeadOpen        bool   `json:"headOpen"`
	FormatsBuffered int    `json:"formatsBuffered"`
	Detail          string `json:"detail"`
}

type previewResult struct {
	PNG        string `json:"png"`
	ZPL        string `json:"zpl"`
	WidthDots  int    `json:"widthDots"`
	LengthDots int    `json:"lengthDots"`
	Dpmm       int    `json:"dpmm"`
}

// Package dymo drives a DYMO LabelWriter through a networked CUPS queue. The
// DYMO is USB-attached to another host (the octoprint Pi); that host shares it
// over IPP, and labl-printr submits the rendered label PNG as an IPP print job.
// CUPS + the DYMO driver handle the raster conversion, so there's no DYMO
// protocol to implement here.
package dymo

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/OpenPrinting/goipp"
)

var client = &http.Client{Timeout: 15 * time.Second}

// Submit sends a PNG label to the CUPS queue at host (default IPP port 631).
// pageSize is the CUPS media name for the loaded die-cut stock (e.g.
// "w81h252"); empty uses the queue default. fit-to-page scales the label onto
// the die-cut label.
func Submit(host, queue, pageSize string, png []byte) error {
	if queue == "" {
		queue = "dymo"
	}
	printerURI := fmt.Sprintf("ipp://%s:631/printers/%s", host, queue)
	postURL := fmt.Sprintf("http://%s:631/printers/%s", host, queue)

	msg := goipp.NewRequest(goipp.DefaultVersion, goipp.OpPrintJob, 1)
	msg.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
	msg.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-us")))
	msg.Operation.Add(goipp.MakeAttribute("printer-uri", goipp.TagURI, goipp.String(printerURI)))
	msg.Operation.Add(goipp.MakeAttribute("requesting-user-name", goipp.TagName, goipp.String("labl-printr")))
	msg.Operation.Add(goipp.MakeAttribute("job-name", goipp.TagName, goipp.String("labl-printr")))
	msg.Operation.Add(goipp.MakeAttribute("document-format", goipp.TagMimeType, goipp.String("image/png")))
	msg.Job.Add(goipp.MakeAttribute("fit-to-page", goipp.TagBoolean, goipp.Boolean(true)))
	if pageSize != "" {
		msg.Job.Add(goipp.MakeAttribute("media", goipp.TagKeyword, goipp.String(pageSize)))
	}

	header, err := msg.EncodeBytes()
	if err != nil {
		return fmt.Errorf("encode ipp: %w", err)
	}
	body := append(header, png...)

	req, err := http.NewRequest(http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/ipp")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("submit to cups at %s: %w", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cups returned HTTP %d", resp.StatusCode)
	}
	var reply goipp.Message
	if err := reply.Decode(resp.Body); err != nil {
		return fmt.Errorf("decode ipp reply: %w", err)
	}
	// IPP status codes below 0x0100 are success/informational.
	if goipp.Status(reply.Code) > 0x00ff {
		return fmt.Errorf("print job rejected: %s", goipp.Status(reply.Code))
	}
	return nil
}

// Reachable reports whether the CUPS server answers on its IPP port.
func Reachable(host string) bool {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:631/", host), nil)
	if err != nil {
		return false
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

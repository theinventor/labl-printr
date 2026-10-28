// Package labelpolicy enforces customer-facing Shopify POS label destinations.
package labelpolicy

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var productPath = regexp.MustCompile(`^/products/[a-zA-Z0-9][a-zA-Z0-9-]*/?$`)
var variantID = regexp.MustCompile(`^[1-9][0-9]*$`)

// Validate runs in the CLI and server before previewing, printing or reprinting.
// Generic inventory labels can still link to private inventory systems.
func Validate(template string, vars map[string]string) error {
	if template != "inventory-shopify-pos" {
		return nil
	}
	u, err := url.Parse(vars["url"])
	if err != nil {
		return fmt.Errorf("Shopify POS QR requires a public HTTPS storefront product URL")
	}
	host := strings.ToLower(u.Hostname())
	private := host == "localhost" || !strings.Contains(host, ".") || net.ParseIP(host) != nil
	for _, suffix := range []string{".ts.net", ".local", ".localhost", ".internal", ".lan", ".home", ".localdomain"} {
		private = private || strings.HasSuffix(host, suffix)
	}
	if u.Scheme != "https" || private || u.Port() != "" || u.User != nil || !productPath.MatchString(u.Path) {
		return fmt.Errorf("Shopify POS QR must use a public HTTPS /products/ storefront URL; VM, Studio, private-network and admin URLs are blocked")
	}
	if values, ok := u.Query()["variant"]; ok && (len(values) != 1 || !variantID.MatchString(values[0])) {
		return fmt.Errorf("Shopify POS QR variant must be one published numeric variant ID")
	}
	return nil
}

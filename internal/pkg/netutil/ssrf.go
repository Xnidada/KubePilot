package netutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateOutboundURL rejects non-http(s) schemes and private/link-local/loopback targets (SSRF guard).
func ValidateOutboundURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || lower == "metadata.google.internal" {
		return fmt.Errorf("url host is not allowed")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("url resolves to a private or link-local address")
		}
		return nil
	}

	// Resolve hostname and ensure no private addresses.
	addrs, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("failed to resolve host: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("host has no DNS records")
	}
	for _, ip := range addrs {
		if isBlockedIP(ip) {
			return fmt.Errorf("url resolves to a private or link-local address")
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS/GCP/Azure metadata
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	return false
}

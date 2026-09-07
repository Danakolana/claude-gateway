package security

import (
	"net"
	"strings"
)

// IsMetadataOrLinkLocalIP blocks cloud metadata and link-local addresses.
func IsMetadataOrLinkLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}

// IsLoopbackIP reports loopback addresses (allowed for local gateways like 9router).
func IsLoopbackIP(ip net.IP) bool {
	return ip != nil && ip.IsLoopback()
}

// IsPrivateNonLoopbackIP reports RFC1918 / ULA addresses excluding loopback.
func IsPrivateNonLoopbackIP(ip net.IP) bool {
	return ip != nil && ip.IsPrivate() && !ip.IsLoopback()
}

// IsBlockedHostname rejects metadata-style hostnames.
// localhost is allowed for local OpenAI-compatible gateways (9router).
func IsBlockedHostname(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	switch h {
	case "metadata.google.internal", "metadata":
		return true
	}
	return false
}

// IsLocalHostname reports localhost aliases.
func IsLocalHostname(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "localhost" || strings.HasSuffix(h, ".localhost")
}

// ClassifyURLHost returns "ok", "loopback", "private", or "blocked".
func ClassifyURLHost(host string) string {
	if IsBlockedHostname(host) {
		return "blocked"
	}
	if IsLocalHostname(host) {
		return "loopback"
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsMetadataOrLinkLocalIP(ip) {
			return "blocked"
		}
		if IsLoopbackIP(ip) {
			return "loopback"
		}
		if IsPrivateNonLoopbackIP(ip) {
			return "private"
		}
	}
	return "ok"
}

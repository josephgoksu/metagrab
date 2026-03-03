package metagrab

import (
	"fmt"
	"net"
	"net/url"
)

// DenyPrivateIPs returns a URLPolicy that resolves the URL hostname and
// rejects any address that maps to a private, loopback, link-local, or
// otherwise reserved IP range. Use it to prevent SSRF when exposing
// metagrab as a service.
//
// Limitation: this check resolves DNS before the HTTP request. A DNS
// rebinding attack (first lookup → public IP, second lookup → 127.0.0.1)
// can bypass it. For hardened deployments, combine this with a custom
// http.Transport that validates resolved IPs in its DialContext.
func DenyPrivateIPs() URLPolicy {
	return func(u *url.URL) error {
		host := u.Hostname()
		ips, err := net.LookupHost(host)
		if err != nil {
			return fmt.Errorf("failed to resolve host %q: %w", host, err)
		}
		for _, ip := range ips {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if parsed.IsLoopback() || parsed.IsPrivate() ||
				parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() ||
				parsed.IsUnspecified() {
				return fmt.Errorf("host %q resolved to private/reserved IP %s", host, ip)
			}
		}
		return nil
	}
}

package metagrab

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// ErrPrivateAddress is returned (wrapped) when a request targets an address
// in a private, loopback, link-local or otherwise reserved range.
var ErrPrivateAddress = errors.New("address is private or reserved")

// reservedPrefixes lists ranges that net/netip helpers do not already cover.
var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT (RFC 6598)
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved + broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 (can reach IPv4 private space)
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
}

// IsPublicAddr reports whether addr is a globally routable unicast address.
// It returns false for loopback, private (RFC 1918, ULA), link-local,
// multicast, unspecified, CGNAT, documentation, NAT64 and other reserved
// ranges. IPv4-mapped IPv6 addresses are checked as IPv4.
func IsPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsLoopback() ||
		addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsMulticast() {
		return false
	}
	for _, p := range reservedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// DenyPrivateIPs returns a URLPolicy that resolves the URL hostname and
// rejects it when any resolved address is not public (see IsPublicAddr).
//
// A URLPolicy only runs once, before the first request. It does not see
// redirects, and a DNS rebinding attack can return a public address to the
// policy and a private one to the dialer. When exposing metagrab as a
// service, use WithDenyPrivateNetworks instead: it checks every connection,
// including redirects, at dial time.
func DenyPrivateIPs() URLPolicy {
	return func(u *url.URL) error {
		host := u.Hostname()
		if addr, err := netip.ParseAddr(host); err == nil {
			if !IsPublicAddr(addr) {
				return fmt.Errorf("host %q: %w", host, ErrPrivateAddress)
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return fmt.Errorf("failed to resolve host %q: %w", host, err)
		}
		for _, a := range addrs {
			if !IsPublicAddr(a) {
				return fmt.Errorf("host %q resolved to %s: %w", host, a, ErrPrivateAddress)
			}
		}
		return nil
	}
}

// dialControlDenyPrivate is a net.Dialer Control hook that refuses to
// connect to non-public addresses. It runs after DNS resolution, for every
// connection attempt, so it also covers redirects and DNS rebinding.
func dialControlDenyPrivate(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("dial %q: %w", address, err)
	}
	if !IsPublicAddr(ap.Addr()) {
		return fmt.Errorf("dial %s: %w", ap.Addr(), ErrPrivateAddress)
	}
	return nil
}

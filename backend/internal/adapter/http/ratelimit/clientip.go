package ratelimit

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// DefaultTrustedProxies are the private and loopback ranges, where the
// compose networks of Caddy, nginx and the API live. They are safe only
// while the API's own port is not published (ADR 0021).
func DefaultTrustedProxies() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("::1/128"),
	}
}

// ClientIP finds the client's address behind trusted proxies (ADR 0021).
type ClientIP struct {
	trusted []netip.Prefix
}

// NewClientIP returns the rule that trusts the proxies in trusted.
func NewClientIP(trusted []netip.Prefix) ClientIP {
	return ClientIP{trusted: append([]netip.Prefix(nil), trusted...)}
}

// Of returns the client's address for r, and false only when even the
// peer's address cannot be read, which net/http never gives a real request:
//   - a peer that is not a trusted proxy is the client, whatever
//     X-Forwarded-For says;
//   - otherwise X-Forwarded-For is walked from the right, skipping trusted
//     proxies, and the first other address is the client;
//   - an entry that is not an address, or a header of trusted proxies
//     only, stops the walk at the last trusted hop. A proxy never writes a
//     malformed entry, so it came from outside, and stopping keeps it from
//     choosing the key.
//
// The leftmost value is never taken on its own: the client writes it.
func (c ClientIP) Of(r *http.Request) (netip.Addr, bool) {
	peer, ok := parseHostPort(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, false
	}
	if !c.isTrusted(peer) {
		return peer, true
	}
	last := peer
	hops := forwardedFor(r.Header)
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return last, true
		}
		addr = addr.Unmap().WithZone("")
		if !c.isTrusted(addr) {
			return addr, true
		}
		last = addr
	}
	return last, true
}

// Value is the key value for the client of r: an IPv4 address as is, an
// IPv6 one as its /64, which one subscriber usually holds whole. It is ""
// when the peer cannot be read.
func (c ClientIP) Value(r *http.Request) string {
	addr, ok := c.Of(r)
	if !ok {
		return ""
	}
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}

func (c ClientIP) isTrusted(addr netip.Addr) bool {
	for _, p := range c.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// forwardedFor returns the X-Forwarded-For entries in order, every header
// line joined, as RFC 9110 reads repeated fields.
func forwardedFor(h http.Header) []string {
	var hops []string
	for _, line := range h.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(line, ",")...)
	}
	return hops
}

func parseHostPort(hostport string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap().WithZone(""), true
}

package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// HeaderForwardedFor carries the client address across the reverse proxy.
const HeaderForwardedFor = "X-Forwarded-For"

// ClientIP is the address every per-IP decision is keyed by: the rate limit of
// tech.md §9.6 and the address recorded on an admin session.
//
// In production the only peer the application ever sees is Caddy, so RemoteAddr
// alone would collapse every visitor into one bucket and make a per-IP limit a
// per-shop limit. X-Forwarded-For carries the real address, but it is a request
// header and anyone can write it, so it is read only when the peer is a proxy
// we could plausibly be sitting behind: a loopback or private address, which on
// this deployment means the compose network and nothing reachable from outside.
// A request that arrives straight from the internet keeps its RemoteAddr no
// matter what it claims in the header.
func ClientIP(r *http.Request) string {
	peer := hostOf(r.RemoteAddr)
	if !isTrustedHop(peer) {
		return peer
	}
	// Walk the chain from the right: every trailing entry is a hop we trust to
	// have appended the one before it, and the first address that is not such a
	// hop is the furthest point we can still believe.
	forwarded := r.Header.Get(HeaderForwardedFor)
	for i := len(forwarded); i > 0; {
		start := strings.LastIndexByte(forwarded[:i], ',') + 1
		candidate := strings.TrimSpace(forwarded[start:i])
		i = start - 1

		addr, err := netip.ParseAddr(candidate)
		if err != nil {
			// A malformed entry means the chain cannot be trusted past this
			// point; stop rather than reach for an older, easier-to-forge one.
			break
		}
		if !isTrustedHop(addr.String()) {
			return addr.String()
		}
	}
	return peer
}

// hostOf drops the port RemoteAddr carries. A value without one is returned as
// it is: it is still a better limiter key than an empty string.
func hostOf(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// isTrustedHop reports whether an address can only belong to our own network.
func isTrustedHop(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

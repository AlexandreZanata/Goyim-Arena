// Package clientip answers one question for the process: which network
// address did this request come from (P16-T03).
//
// The question looks trivial and is the source of a whole class of abuse
// bypasses. A request carries two kinds of network fact: the address the
// kernel actually saw (http.Request.RemoteAddr, written by net/http from the
// accepted connection) and whatever the client put in a forwarding header
// (X-Forwarded-For and friends, fully attacker-controlled). Only the first is
// evidence. The second becomes evidence only when the address that sent it is
// a proxy this deployment has declared trusted, because then the proxy wrote
// it, not the client.
//
// So the rule here is inverted from the naive one. The default configuration
// trusts nobody, which means every forwarding header is ignored and the key is
// the peer address: a client that rotates X-Forwarded-For to look like a
// thousand clients is one client. When trusted proxies are configured, the
// header is read with chain semantics — the standard that lets the application
// verify the last hop by itself:
//
//   - the immediate peer must be inside a trusted prefix, or no header is
//     consulted at all;
//   - X-Forwarded-For is walked right to left, skipping addresses that are
//     themselves trusted proxies; the first address that is not a trusted
//     proxy is the client, because that is the entry the last trusted hop
//     appended from what it actually observed;
//   - if every entry is a trusted address, the leftmost one is taken; if the
//     header is absent, unparseable or empty, the peer is the answer.
//
// X-Real-IP and CF-Connecting-IP are deliberately not consulted. They carry a
// single value with no chain, so an application behind a proxy that forwards
// client headers cannot tell a value written by the proxy from one written by
// the client — the header would be trusted on the strength of the peer alone,
// which is exactly the spoof this package exists to prevent. X-Forwarded-For
// chains give the application a fact it can check itself: the entry the
// trusted hop appended.
//
// What this package does not do: it makes no claim about abuse. An address is
// an imperfect signal and potential personal data (docs/SECURITY.md, section
// 6); it is a key for bounded accounting, never proof, and the addresses it
// returns live in memory for the lifetime of a rate limit window, not in a
// log, a metric or the database.
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// maxForwardedEntries bounds how many X-Forwarded-For entries are parsed, and
// the window retained is the *rightmost* ones: the entries appended by the
// proxies closest to this process, which are the ones a trusted hop wrote. The
// leftmost entries are the client's own claim, so padding the header is how an
// attacker would try to push the verifiable end out of the window; keeping the
// tail is what makes the bound unable to help them.
const maxForwardedEntries = 32

// forwardedHeader is the only forwarding header consulted, for the reason
// documented on the package.
const forwardedHeader = "X-Forwarded-For"

// Resolver maps a request to the address of its client, trusting forwarding
// headers only from configured proxies. The zero value trusts nobody, which is
// the safe default.
type Resolver struct {
	trusted []netip.Prefix
}

// New builds a resolver from the trusted proxy prefixes. An empty set is
// valid and is the recommended default: no forwarding header is honored.
func New(trusted []netip.Prefix) Resolver {
	return Resolver{trusted: append([]netip.Prefix(nil), trusted...)}
}

// ParseTrusted builds prefixes from operator input: "10.0.0.0/8",
// "2001:db8::/32", or a bare address, which becomes a single-host prefix
// (/32 or /128). Addresses are canonicalized, so an IPv4-mapped IPv6 literal
// matches its IPv4 form.
func ParseTrusted(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(value); err == nil {
			prefixes = append(prefixes, normalizePrefix(prefix))
			continue
		}
		address, err := netip.ParseAddr(value)
		if err != nil {
			return nil, &InvalidTrustedProxyError{Value: raw}
		}
		address = address.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(address, address.BitLen()))
	}
	return prefixes, nil
}

// InvalidTrustedProxyError reports operator input that is neither a CIDR nor
// an address.
type InvalidTrustedProxyError struct {
	Value string
}

// Error renders the invalid value. The value is operator input, not client
// input, so naming it is safe and is what makes the failure fixable.
func (err *InvalidTrustedProxyError) Error() string {
	return "clientip: trusted proxy " + err.Value + " is neither a CIDR nor an IP address"
}

// Trusts reports whether the address is inside a configured trusted prefix.
func (resolver Resolver) Trusts(address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	address = address.Unmap()
	for _, prefix := range resolver.trusted {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// Count reports how many trusted prefixes are configured. It exists so
// composition can be asserted (and so a deployment can tell, in a test, that
// the trusted set it intended is the one in force).
func (resolver Resolver) Count() int {
	return len(resolver.trusted)
}

// Client returns the client address of the request.
//
// The returned address is always valid for a request that came from a real
// connection; it is the zero address only when RemoteAddr is absent or
// unparseable, which the caller must treat as "unknown" and bound accordingly
// rather than as "unidentified, therefore unlimited".
func (resolver Resolver) Client(request *http.Request) netip.Addr {
	peer, ok := parseAddress(request.RemoteAddr)
	if !ok {
		return netip.Addr{}
	}
	if !resolver.Trusts(peer) {
		return peer
	}

	forwarded := parseForwarded(request.Header.Get(forwardedHeader))
	for index := len(forwarded) - 1; index >= 0; index-- {
		if !resolver.Trusts(forwarded[index]) {
			return forwarded[index]
		}
	}
	if len(forwarded) > 0 {
		return forwarded[0]
	}
	return peer
}

// parseAddress reads one host:port or bare address, canonicalizing IPv4-mapped
// IPv6 into IPv4 so "::ffff:198.51.100.7" and "198.51.100.7" are one client
// instead of two.
func parseAddress(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Addr{}, false
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	address, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

// parseForwarded reads the X-Forwarded-For list, dropping entries that are not
// addresses. A malformed entry is skipped rather than aborting the walk: it
// cannot be a trusted proxy, so the entries after it keep their position.
func parseForwarded(header string) []netip.Addr {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	if len(parts) > maxForwardedEntries {
		parts = parts[len(parts)-maxForwardedEntries:]
	}
	addresses := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		// Some proxies append "address:port"; parseAddress accepts both.
		if address, ok := parseAddress(part); ok {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

// normalizePrefix canonicalizes a prefix: the network bits are masked, so
// "10.0.0.7/8" behaves like "10.0.0.0/8" instead of silently matching a
// narrower set, and an IPv4-mapped IPv6 prefix becomes its IPv4 form, so a
// mapped literal matches the same clients as the IPv4 prefix would.
func normalizePrefix(prefix netip.Prefix) netip.Prefix {
	if !prefix.IsValid() {
		return prefix
	}
	prefix = prefix.Masked()

	address := prefix.Addr()
	bits := prefix.Bits()
	if address.Is4In6() {
		if bits < 96 {
			// A prefix that spans the whole mapped range has no IPv4 form.
			return prefix
		}
		address = address.Unmap()
		bits -= 96
	}
	if bits > address.BitLen() {
		bits = address.BitLen()
	}
	return netip.PrefixFrom(address, bits)
}

package egress

import "net/netip"

// denied are the ranges of Denied beyond those netip.Addr reports.
var denied = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
}

// Denied reports whether a is on the base list of addresses no app may reach: unspecified,
// loopback, private, link-local, multicast, shared (CGNAT), documentation, benchmarking,
// reserved and broadcast addresses, and the IPv6 ranges that carry IPv4 addresses (NAT64,
// Teredo, 6to4). Cloud metadata addresses, such as 169.254.169.254 and fd00:ec2::254, fall in
// these ranges. An IPv4-mapped IPv6 address is checked as its IPv4 address, an address with a
// zone without it, and the zero Addr is denied.
func Denied(a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	if !a.IsValid() || a.IsUnspecified() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsMulticast() {
		return true
	}
	for _, p := range denied {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

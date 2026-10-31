# E-OUT-003: address no app may reach

The host of the app's request resolves to an address that no app may reach, or is one of the
platform's own hosts. The runner refuses unspecified, loopback, private, link-local, multicast,
shared (CGNAT), cloud metadata, documentation, benchmarking and reserved addresses, the IPv6
ranges that carry IPv4 addresses (NAT64, 6to4, Teredo), the addresses of the server the app runs
on and the platform's other addresses, with no exceptions. It resolves the host each time it
connects: when any address of the answer is refused, the request is refused, and it connects only
to an address of an answer it checked, so a host whose answer changes is checked again. Nothing
was sent.

**Fix:** call a public service; the platform's own hosts and private networks are never reachable

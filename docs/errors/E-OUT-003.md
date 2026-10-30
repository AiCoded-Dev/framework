# E-OUT-003: address no app may reach

The host of the app's request resolves to an address that no app may reach, or is one of the
platform's own hosts. The runner refuses loopback, private, link-local, shared (CGNAT), cloud
metadata and documentation addresses, the addresses of the server the app runs on and the
platform's other addresses, with no exceptions. When the host resolves to several addresses and
any of them is refused, the request is refused. Nothing was sent.

**Fix:** call a public service; the platform's own hosts and private networks are never reachable

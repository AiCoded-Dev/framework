# E-OUT-001: host not in egress

The app sent a request with `outbound.Client()` to a host that `egress` in its permission list
(`aicoded.yaml`) does not list. The runner compares the URL's host, in lower case and without a
dot at its end, with the hosts in `egress` exactly: a subdomain, an IP address or a host written
with letters outside ASCII is a different host. Write an international name in its `xn--` form.
Nothing was sent.

A preview reaches only the hosts of the app's live release, with the same key bindings: a host
the preview's publish adds, or binds another way, waits for the release.

A new host is a change of the permission list: one the app type does not allow needs approval.

**Fix:** add the host to egress in aicoded.yaml; a host the app type does not allow needs approval

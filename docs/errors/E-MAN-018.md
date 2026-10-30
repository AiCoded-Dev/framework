# E-MAN-018: invalid egress host

`egress` in the permission list (`aicoded.yaml`) lists the outside services the app calls, by host name; the app reaches them over HTTPS on port 443. An entry is the host alone, or a mapping whose `host` is the host and which binds a key to it (see E-MAN-025). Each host is a lower-case DNS name, such as `api.partner.example`: no scheme, port, path or wildcard, no IP address and no dot at the end. A name that leads to this computer or a private network is refused: a name whose last label is `localhost`, `local`, `internal` or `arpa`, such as `metadata.google.internal`. So is a host of the platform itself, such as `aicoded.cloud`, `aicoded.dev` or a subdomain of one, in any case. Each host is listed once, alone or with a key, and a mapping without a `host` is refused.

```yaml
egress: [https://api.partner.example/v1]   # wrong: a URL
egress: [api.aicoded.cloud]                # wrong: a host of the platform
egress: [api.partner.example]              # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** write the host alone, lower-case, such as `api.partner.example`: no scheme, port, path, wildcard or IP address, nothing local, each once.

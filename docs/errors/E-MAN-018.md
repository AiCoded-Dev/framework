# E-MAN-018: invalid egress host

`egress` in the permission list (`aicoded.yaml`) lists the outside services the app calls, by host name; the app reaches them over HTTPS on port 443. Each entry is a lower-case DNS name, such as `api.partner.example`: no scheme, port, path or wildcard, no IP address and no dot at the end. A name that leads to this computer or a private network is refused: names ending in `localhost`, `local`, `internal` or `arpa`, such as `metadata.google.internal`. Each host is listed once.

```yaml
egress: [https://api.partner.example/v1]   # wrong: a URL
egress: [api.partner.example]              # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** write the host alone, lower-case, such as `api.partner.example`: no scheme, port, path, wildcard or IP address, nothing local, each once.

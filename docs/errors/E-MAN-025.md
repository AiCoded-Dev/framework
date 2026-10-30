# E-MAN-025: invalid key binding in egress

An entry of `egress` in the permission list (`aicoded.yaml`) binds a key to an outside service the wrong way. The runner adds a bound key to every request the app sends to the host with `outbound.Client()`, so the app never reads it. A binding is a mapping with these fields:

- `host`: the outside service's host, as in E-MAN-018;
- `secret`: the name of a secret declared in `secrets`, which holds the key;
- `header`: the request header the key goes in, a header name of at most 64 characters: letters, digits and ``!#$%&'*+-.^_`|~``. It may not be `Host`, `Content-Length`, `Content-Type`, `Cookie`, or a header of the connection itself: `Connection`, `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `Proxy-Connection`, `TE`, `Trailer`, `Transfer-Encoding` or `Upgrade`, in any case;
- `prefix`, which may be left out: the text the runner puts before the key, at most 64 printable ASCII characters, spaces included.

The message says which of these is wrong: a secret that `secrets` does not declare, a secret with no header, a header or a prefix with no secret, a header that is not a header name or may not carry a key, or a prefix that is too long or has other characters. One secret may be bound to several hosts.

```yaml
egress:
  - host: api.partner.example
    secret: partner_api_key      # declared below
    header: Authorization
    prefix: "Bearer "
secrets: [partner_api_key]
```

See [the permission list](../guides/permission-list.md).

**Fix:** bind a declared secret to a header such as Authorization, with an optional prefix such as "Bearer ".

# E-OUT-009: header the runner refuses

The app sent a request with `outbound.Client()` whose method or header name is not an HTTP token
(letters, digits and ``!#$%&'*+-.^_`|~``), or a header value with a line break or another control
character. A key that `egress` binds to the host may not hold one either. Nothing was sent.

The runner drops the headers of the connection itself and `Host`, and replaces every header of a
bound key's name that the app set.

**Fix:** use header names made of letters, digits and dashes, and values without line breaks

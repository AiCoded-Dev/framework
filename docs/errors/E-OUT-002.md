# E-OUT-002: not https on port 443

The app sent a request with `outbound.Client()` to a URL the runner does not send: one that is not
`https://`, names a port other than 443, holds a user name or password before its host, or is not
a full URL with a host. Nothing was sent.

The runner also sends only the methods GET, HEAD, POST, PUT, PATCH, DELETE and OPTIONS; any other,
such as CONNECT or TRACE, is refused with the fix "send the request with GET, HEAD, POST, PUT,
PATCH, DELETE or OPTIONS".

**Fix:** call the service over https on port 443

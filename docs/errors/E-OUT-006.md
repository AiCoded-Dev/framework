# E-OUT-006: request took too long

A request to an outside service has 30 seconds in all: resolving the host, connecting, sending
the request and reading the whole response. The outside service took longer, so the runner ended
the request. The service may have acted on it. A host that does not resolve within 5 seconds is
E-OUT-008 instead.

**Fix:** ask the service for less, or try again later

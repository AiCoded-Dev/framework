# E-RUN-007: enrolment refused

A server's runner enrols once with the platform: the operator makes a one-time enrolment token for
the server, and the runner sends it with a request for a certificate for a key it made itself. Later
it renews that certificate the same way, over its connection to the platform. The platform refused
the request: the token is unknown, was used already, expired after its hour, or was made for another
server; or the request for the certificate names something other than this server's runner, or uses
a key of a kind the platform does not take. Nothing was issued. A token that a refused request
carried stays unused, until it expires.

**Fix:** make a new token with `admin servers enroll <server> --out <file>`, and let `aicoded-runner enroll` make a new key and request with it

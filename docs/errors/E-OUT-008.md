# E-OUT-008: outside service not reached

The runner could not reach the outside service: its host name did not resolve within 5 seconds,
the connection failed, or its TLS certificate is not valid for the host. The reason is not passed
on, since it can name addresses of the network the app runs in. The request may not have been
sent. The app also gets this code when it cancelled the request itself; the message says so.

**Fix:** check the host name, or try again later

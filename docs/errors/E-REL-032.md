# E-REL-032: the platform could not sign the release

The control plane could not sign a release or a preview of the app, and nothing was released or
started. The server's runner starts only a release or a preview that carries the platform's
signature, so one the platform cannot sign is never sent. A publish that passed before the platform
decided publishes has no permission-list hash to sign, so it is never signed. The control plane
refuses to start, with this code, when its signing key is missing or invalid.

**Fix:** Try again; if it persists, the operator checks the control plane's signing key. For a
publish that passed before the platform decided publishes, publish the commit again and use the new
publish.

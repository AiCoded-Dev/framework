# E-RUN-008: runner certificate not valid

A server's runner connects to the platform with the certificate it got when it enrolled, and the
platform serves it only the apps of the server that certificate names. The platform refused the
certificate: the runner presented none, one the platform did not issue, one that expired, or one the
operator revoked with `admin servers revoke`; or the runner said it is another server than
its certificate names. A certificate also stops working once the runner connects with the one that
renewed it. The runner keeps its apps serving while it cannot connect, but it starts no new release
and lets no new viewer sign in until it enrols again.

**Fix:** enrol the server's runner again: make a token with `admin servers enroll <server> --out <file>`, then run `aicoded-runner enroll` with it

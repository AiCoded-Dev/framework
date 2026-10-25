# E-RUN-008: runner certificate not valid

A server's runner connects to the platform with the certificate it got when it enrolled, and the
platform serves it only the apps of the server that certificate names. The platform refused the
certificate: the runner presented none, one the platform did not issue, one that expired, as after
the runner was offline for more than its 30 days, or one the operator revoked with
`admin servers revoke`; or the runner said it is another server than its certificate names. A
certificate also stops working once the runner connects with the one that renewed it. While it
cannot connect, the runner keeps its apps' instances running, but it starts no new release and lets
no new viewer sign in until it enrols again. A revoked runner keeps serving the viewers who are
signed in. An expired certificate, though, also stops the gateway reaching the runner, so every app
of the server answers its viewers 502 until the server is enrolled again: enrol it at once. A
runner renews its certificate over its connection, so one that connects holds a certificate with 10
to 30 days left: a platform that runners cannot reach for more than 10 days may let a server's
certificate expire, and for more than 30 days lets every server's expire.

**Fix:** enrol the server's runner again: make a token with `admin servers enroll <server> --out <file>`, then run `aicoded-runner enroll` with it

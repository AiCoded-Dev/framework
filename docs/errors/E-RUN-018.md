# E-RUN-018: enrolment by the instance role refused

A server In your own cloud runs its runner on an EC2 instance of the guest zone that the customer
installed in their AWS account. That runner has no enrolment token: it enrols by sending the
platform a request to AWS STS that the instance's role signed, for its target, and a request for a
certificate for a key it made itself, whose SHA-256 the signed request carries too, so that no one
can send the same signed request with another certificate request. The platform refused it: the
signed request is not one to STS's `GetCallerIdentity`, was signed more than 5 minutes from the
platform's clock, is for another target, or does not sign the SHA-256 of the certificate request
sent with it; STS did not take it; it was signed by another role than the guest zone's
`aicoded-runner-host-<target>` in the target's account, or not in the session of the instance; the
instance is not running or not tagged `aicoded:target` with the target; or a runner of that
instance already holds a certificate that is neither expired nor revoked. Nothing was issued.

**Fix:** check that the runner runs on a running instance of the target's guest zone, with the role `aicoded-runner-host-<target>`, the tag `aicoded:target=<target>` and a right clock, then start the runner again; an instance whose runner lost its certificate enrols again once the operator revoked the server's certificates with `admin servers revoke <server>`

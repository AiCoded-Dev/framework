# E-REL-033: the platform cannot sign in to the target In your own cloud

The organisation's apps run In your own cloud: in a guest zone that the customer installed in
their AWS account. The platform makes each app's database, secrets and logs there by signing in to
the guest zone's role `aicoded-control-plane` with the external ID the platform's operator gave
the customer. AWS refused that sign-in, as when the guest zone's stack is not installed yet, was
removed, or was installed with another external ID. Nothing was made and nothing was released.

**Fix:** the customer checks that the guest zone's stack is installed in the account with the
external ID the platform's operator gave them, and the operator checks the control plane's `aws`
section; then release again

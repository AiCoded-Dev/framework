# E-REL-034: the guest zone In your own cloud is not ready

The organisation's apps run In your own cloud, in a guest zone that the customer installed in
their AWS account from the platform's template. The guest zone lacks something the platform needs
to make the app's resources or release it:

- its stack has not finished installing, or has no database outputs, no `FilesBucket` output that
  names the bucket of the file stores, no `MailBucket` output while it receives mail, no output
  that names its group of hosts, or no log retention;
- an IAM role of the app exists that the platform did not make, outside the app's path or without
  the app boundary. The platform leaves such a role alone;
- the role provisioner, a small function in the customer's account that makes the app's roles,
  refused, or answered a role that is not the app's;
- the mail provisioner, the function that makes the receipt rule of the app's inbox, refused,
  answered a rule that is not the app's, is missing, or may not be invoked by the platform;
- the platform's role may not manage the mail domains in SES.

A missing output or function, and a right the platform's role lacks, mostly mean that the stack
was installed from a template older than the platform's. Nothing was released.

**Fix:** the customer finishes installing the guest zone's stack, or updates it to the platform's
template, removes an app role the platform did not make, and looks at the role or mail
provisioner's log in their account when it refused; then release again

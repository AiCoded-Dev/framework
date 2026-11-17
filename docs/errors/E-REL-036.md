# E-REL-036: the mail domain In your own cloud has another MAIL FROM domain

The organisation's apps run In your own cloud, where SES in the customer's AWS account sends their
mail. The platform sends each mail domain's mail from its own MAIL FROM domain, `bounce.<domain>`,
so that receiving servers check SPF and DMARC against the customer's domain. The domain's identity
in SES already has another MAIL FROM domain, which someone set in the customer's account. The
platform never changes a MAIL FROM domain it did not set, so it neither adds, checks nor sends from
the domain. When the company's own mail relay sends the apps' mail, SES sends none, and the
platform does not look at the MAIL FROM domain. Nothing was released.

**Fix:** the customer sets the identity's MAIL FROM domain to `bounce.<domain>`, choosing Reject
message for when its MX record is missing, or clears it, in the SES console of the guest zone's
account and region; then add or check the mail domain, or release, again

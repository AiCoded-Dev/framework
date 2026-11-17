# E-REL-006: mail domain not verified

The app sends mail from the address in `email.from`. The platform sends mail only from a domain
that the organisation verified with it, so that the servers that receive the mail trust it.
The domain of this address is not one of them, or, In your own cloud, it is one of them but is not
ready in SES in the customer's AWS account yet, and the message says which part: SES has not
verified the domain for sending, has not found its three DKIM records, or has not found the
records of its MAIL FROM domain `bounce.<domain>`, an MX record and an SPF TXT record, which the
platform may have just set. Until SES finds those, it rejects the domain's mail. When the company's
own mail relay sends the apps' mail, SES sends none, and the domain needs no records at
`bounce.<domain>`. A rollback is not refused for this. Nothing was released.

**Fix:** use an `email.from` address in a domain your organisation verified, or ask your administrator to verify the domain with the platform, then commit, publish and release again; In your own cloud, when the domain is verified already, ask your administrator to add the DNS records the message lists, or to look at the domain in SES when SES has not verified it, and release again once SES has found them: `admin mail-domains verify` shows what the target finds now

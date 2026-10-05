# E-MAIL-003: recipient or Reply-To domain not allowed

The app may send mail only to the domains its permission list (`aicoded.yaml`) names in `email.to_domains`, and Reply-To must be in one of them or be the app's own address. Sending to another domain, including a subdomain that is not listed, is refused. Once apps are published through the delivery pipeline, which comes later, a new domain in the list will be a change that security approves.

**Fix:** send only to the domains in `email.to_domains`; to mail another domain, add it to `email.to_domains`.

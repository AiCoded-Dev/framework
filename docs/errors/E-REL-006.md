# E-REL-006: mail domain not verified

The app sends mail from the address in `email.from`. The platform sends mail only from a domain
that the organisation verified with it, so that the servers that receive the mail trust it.
The domain of this address is not one of them. Nothing was released.

**Fix:** use an `email.from` address in a domain your organisation verified, or ask your administrator to verify the domain with the platform; then commit, publish and release again

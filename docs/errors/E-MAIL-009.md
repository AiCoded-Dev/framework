# E-MAIL-009: no hosting for mail domains

The platform's operator asked to add or verify a mail domain of an organisation. The platform
registers each mail domain with the mail service of its hosting, which sends the apps' mail, and
checks the domain's DNS records there. This control plane's configuration names no hosting, so it
can do neither. Nothing was added or verified.

**Fix:** add the hosting section and its service account key to the control plane's configuration, as the platform's README describes, restart the control plane, then add or verify the domain again

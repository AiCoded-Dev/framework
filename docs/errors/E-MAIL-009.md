# E-MAIL-009: no target for mail domains

The platform's operator asked to add or verify a mail domain of an organisation. The platform
registers each mail domain with the mail service of the target of the server the organisation is
placed on, which sends the apps' mail, and checks the domain's DNS records there. The organisation
is placed on no server yet, or the platform cannot reach that server's target, so it can do
neither. Nothing was added or verified.

**Fix:** place the organisation on a server with `admin orgs place <org> <server>`, or make its target reachable as the platform's README describes, then add or verify the domain again

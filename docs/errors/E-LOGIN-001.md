# E-LOGIN-001: account identity too long for company login

Company login signs a person in to an app with the identity that the organisation's sign-in
provider gives them: the provider's issuer and the account's subject, which the app gets joined as
`<issuer>|<subject>`. The platform keeps the issuer and the subject up to 255 characters each, and
the joined identity up to 512 bytes, and never cuts them short, since a shortened identity could be
another person's. This provider named the account with a longer one, so the person was not signed
in. Nothing about the account changed.

**Fix:** sign in with another account, or ask your administrator to have the sign-in provider give accounts shorter subjects

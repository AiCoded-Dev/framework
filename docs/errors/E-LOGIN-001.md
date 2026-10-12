# E-LOGIN-001: account identity too long for company login

Company login signs a person in to an app with the identity that the organisation's sign-in
provider gives them: the provider's issuer and the account's subject. The platform keeps each of
them up to 255 bytes, and never cuts one short, since a shortened identity could be another
person's. This provider named the account with a longer one, so the person was not signed in.
Nothing about the account changed.

**Fix:** sign in with another account, or ask your administrator to have the sign-in provider give accounts subjects of at most 255 bytes

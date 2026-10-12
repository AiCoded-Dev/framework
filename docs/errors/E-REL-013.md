# E-REL-013: mailbox name not taken

Each app sends from a mailbox of its own at the platform's hosting, whose address is the app's
`email.from`. The hosting takes a mailbox name, the part before `@`, of 1 to 64 of `a-z`, `0-9`,
`.`, `_`, `+` and `-`, starting and ending with a letter or digit. The part before `@` of this
address is not such a name. Nothing was released.

**Fix:** give the app an `email.from` address whose part before `@` is lower-case letters, digits, `.`, `_`, `+` and `-`, starting and ending with a letter or digit, such as `rooms@acme.example`; commit, publish and release again

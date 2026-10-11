# E-REL-008: mailbox in use

Each app sends from a mailbox of its own, the address in `email.from`. Another app of the
organisation already has this address, so the platform cannot give it to this app. Nothing was
released.

**Fix:** give the app an `email.from` address of its own, such as `rooms@acme.example` for the app rooms, commit, and publish again

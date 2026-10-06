# E-CLI-004: not signed in to the platform

`aicoded whoami` needs your sign-in to the platform at the address `AICODED_PLATFORM` names, `https://api.aicoded.cloud` by default. There is none for that address: you never signed in to it, you signed out with `aicoded logout`, or the platform no longer accepts or refreshes your sign-in. A sign-in ends when it was revoked, when it was not used for 30 days, or when you are no longer in your organisation's builder group. When the platform refused to refresh it, the message quotes its reason, and `aicoded` deletes the sign-in but remembers the organisation, so `aicoded login` alone signs in again. Each platform address has its own sign-in.

**Fix:** run `aicoded login --org <your organisation>`.

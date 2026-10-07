# E-CLI-009: the platform could not refresh your sign-in right now

Before a command such as `aicoded whoami` sends your sign-in to the platform, `aicoded` refreshes it when it is about to expire, and `aicoded publish` and `aicoded status` refresh it once when the platform does not accept its token, as when this computer's clock is off, or refuses it for want of the scopes of an app you own now, such as the app your first publish created. Here the refresh failed in a way that may pass: `aicoded` could not reach the platform, the platform did not answer in time, or it answered that it, or your organisation's company login, failed for now (`server_error`, `temporarily_unavailable` or a 5xx status). The message says which. Nothing ended your sign-in: `aicoded` keeps it as it was, and the next command tries the refresh again. A sign-in that the platform refuses to refresh for good is E-CLI-004 instead.

**Fix:** try again in a minute: your sign-in is kept.

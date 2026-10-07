# E-PUB-010: the platform refused the bundle

The platform takes a git bundle of version 2 with one ref, which names the commit the publish claims, and with no prerequisite or with the app's base as its only one. It refused this one: its header was not as claimed, or its history started from a commit that was no longer the app's base, because another publish of the app changed the base while this one was sent. When the base changed, `aicoded publish` asks for it again and sends once more; this code comes when it changed again, or when the header was refused.

**Fix:** run `aicoded publish` again.

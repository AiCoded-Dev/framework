# E-CLI-005: the platform's address is not valid

`AICODED_PLATFORM` names the platform that `aicoded login`, `logout` and `whoami` talk to, and they send your sign-in there. It must be an `https` address, or `http://127.0.0.1:<port>` for a platform on this computer, with nothing after the host and port: no user, path, query or fragment. An `https` address with port 443, such as `https://api.aicoded.cloud:443`, is the same platform as the address without it. Plain `http` to any other host would send your sign-in in clear text. The message never quotes the value, which could hold a password.

**Fix:** set `AICODED_PLATFORM` to an `https` address with no path, such as `https://api.aicoded.cloud`, or to `http://127.0.0.1:<port>` for a platform on this computer, or unset it.

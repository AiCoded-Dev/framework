# E-RPC-012: forwarded viewer token refused

A call on behalf of a viewer carries the token of the request the app is serving, and the runner checks it before it passes the viewer on. The token must have been issued by the runner to this app, for a viewer rather than an app, at most 10 minutes ago, which is the longest a live page connection lasts. The app sent a token that fails one of these checks: it was changed or made up, belongs to another app, is an app's own token, or is too old. The message says which.

**Fix:** make calls with the context of the request or call being served (`r.Context()`, or the `ctx` of the hook), never with a context kept from an earlier request.

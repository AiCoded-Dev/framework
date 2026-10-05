# E-RUN-004: no runner in this context

A building block (settings, secrets, files, mail…) was called with a context that did not come
from the app, for example `context.Background()`. Only the app's contexts carry the runner.

**Fix:** pass the request's context (`r.Context()`), or the context `app.Options.OnStart` receives.

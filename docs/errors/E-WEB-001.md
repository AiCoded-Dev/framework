# E-WEB-001: pages served without the runner

The pages handler got a request that did not come through the app's runner, so it has no key to
sign form tokens with and cannot trust the viewer. It answers every such request with 500 and
never falls back to a key of its own.

**Fix:** serve pages through `app.Main(app.Options{Handler: pages.NewHandler(...)})` and start
the app with `aicoded dev`.

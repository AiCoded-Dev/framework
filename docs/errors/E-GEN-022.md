# E-GEN-022: event-handler attribute

An element, an `<ssr:form>` or one of its fields has an attribute whose name starts with `on`,
such as `onclick`. The browser runs its value as script. Pages run under a strict
Content-Security-Policy that blocks inline code, so this would not run in the browser either.

**Fix:** attach the handler in index.ts with `addEventListener`

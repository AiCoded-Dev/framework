# E-GEN-009: nested forms

An `<ssr:form>` is opened inside another `<ssr:form>`. Browsers do not nest forms, so the
inner form's fields would be sent with the outer form and checked against the wrong one.

**Fix:** close the outer `<ssr:form>` before opening another

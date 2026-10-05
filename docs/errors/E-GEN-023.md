# E-GEN-023: `style` attribute

An element, an `<ssr:form>` or one of its fields has a `style` attribute. Pages run under a
strict Content-Security-Policy that blocks inline code, so this would not run in the browser
either.

**Fix:** give the element a class and style it in index.css

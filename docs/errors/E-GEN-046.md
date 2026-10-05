# E-GEN-046: form, content or assets inside a live part of the page

An `<ssr:form>`, `<ssr:content/>` or `<ssr:assets/>` is inside a part of the page that changes
with a live value: an element with `ssr:if`, `ssr:else-if`, `ssr:else` or `ssr:for` that reads
a variable declared with `reactive="true"`, or an element with such a value in an attribute.
When one of its variables changes, the server renders that part again from the page's
variables alone. A form with its hidden security fields, the page below and the page's script
and style tags are not variables, so they cannot be rendered there.

**Fix:** move `<ssr:form>`, `<ssr:content/>` and `<ssr:assets/>` out of the part that changes,
or drop `reactive="true"`

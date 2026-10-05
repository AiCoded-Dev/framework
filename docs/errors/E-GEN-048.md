# E-GEN-048: live value inside a form

An `<ssr:form>` shows a variable declared with `reactive="true"`: in its text, a condition, a
loop, an `<ssr:json>` or an attribute of the form, of an element in it or of a field. A form is
written from the page's form state, which a live update never renders again, so the value would
stay as it was when the page opened. A native input with `ssr:bind` inside a form is not
affected: it is kept in step by the live connection.

**Fix:** show the value outside `<ssr:form>`, or drop `reactive="true"`

# E-GEN-008: form field outside a form

An `<ssr:input>`, `<ssr:select>` or `<ssr:textarea>` is not inside an `<ssr:form>`. Form
fields are read, checked and filled back through their form, so a field outside one would never
be sent or checked.

**Fix:** put `<ssr:input>`, `<ssr:select>` and `<ssr:textarea>` inside an `<ssr:form>`

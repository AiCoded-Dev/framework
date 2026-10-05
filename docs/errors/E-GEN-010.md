# E-GEN-010: invalid `<ssr:form>`

The form has no `name`, a name that is not made of letters, digits and `_` starting with a
letter, the same name as another form in the template, or an `enctype` the form fields cannot
read. The name identifies the form when it is sent and becomes part of the generated Go names.

**Fix:** give the form a `name` made of letters and digits; enctype is `application/x-www-form-urlencoded` or `multipart/form-data`

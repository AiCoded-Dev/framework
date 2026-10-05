# E-GEN-029: raw markup outside element text

A `{{$ }}` is in an attribute value or inside `<title>` or `<textarea>`. `{{$ }}` writes markup as
it is, which works only between elements: in an attribute it could end the attribute, and inside
`<title>` or `<textarea>` the browser shows it as text.

**Fix:** use `{{ }}`; `{{$ }}` works only in element text

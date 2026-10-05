# E-GEN-033: invalid `<ssr:json>`

An `<ssr:json>` has no `value`, writes its value inside `{{ }}`, has an attribute other than
`name` and `value`, has a `name` that is not lower-case letters, digits and `-` starting with
a letter, at most 63 characters long, or has the name of another `<ssr:json>` of the template.
The name becomes the `id` of the `<script type="application/json">` element that holds the value
as JSON, where page scripts read it, so each name is used once.

**Fix:** write `<ssr:json name="lower-case-id" value="expression"/>`

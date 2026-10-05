# E-GEN-018: `ssr:bind` on a form field or an element that is not an input

An `<ssr:input>`, `<ssr:select>` or `<ssr:textarea>` has `ssr:bind`. A form field takes its value
from its form, and a live binding would set the same element from a live variable, so the two
would overwrite each other. `ssr:bind` on any other element than a native `<input>`, `<select>`
or `<textarea>`, such as a `<div>`, is refused too: only an input has a value the viewer can
change. So is `ssr:bind` on an `<input type="file">`, whose file a script cannot set, and on an
`<input>` whose `type` is a `{{ }}` value, which could make it one.

**Fix:** use a native `<input>`, `<select>` or `<textarea>` for live binding

# E-GEN-034: live value in `<title>` or `<textarea>`

A `{{ }}` inside `<title>` or `<textarea>` reads a variable declared with `reactive="true"`.
The page updates a live value through an element around it, and `<title>` and `<textarea>`
hold only text, so that element would show as text. To keep a `<textarea>` in step with a
variable, give it `ssr:bind` instead.

**Fix:** show the value elsewhere, or drop `reactive="true"`

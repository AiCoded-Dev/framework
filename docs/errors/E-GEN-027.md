# E-GEN-027: value in an attribute that loads code

A `{{ }}` is in an attribute of `<script>`, `<base>`, `<meta>`, `<link>`, `<object>`, `<embed>`,
`<applet>` or an SVG animation element (`<animate>`, `<set>`, `<animateMotion>`,
`<animateTransform>`), or in `srcdoc`, `srcset`, `imagesrcset` or `http-equiv` on any element.
These attributes load code, change how the page is read or set other attributes, so their values
are fixed in the template.

**Fix:** use a fixed value for this attribute

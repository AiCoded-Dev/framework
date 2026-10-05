# E-GEN-005: unknown `ssr:` attribute

An element has an attribute with the `ssr:` prefix that the generator does not know, or an
`ssr:` tag has an `ssr:` attribute. Attributes with this prefix are instructions to the
generator, so an unknown one is refused rather than sent to the browser. `ssr:` attributes do
not work on `ssr:` tags; put them on an element around the tag.

**Fix:** use `ssr:if`, `ssr:else-if`, `ssr:else`, `ssr:for` or `ssr:bind`

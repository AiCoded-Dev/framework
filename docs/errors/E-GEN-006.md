# E-GEN-006: `ssr:else` without `ssr:if`

An element with `ssr:else` or `ssr:else-if` does not directly follow an element with `ssr:if`
or `ssr:else-if`, or it follows an element that already has `ssr:else`. The generator cannot
tell which condition it continues. It is also raised when one element has two of `ssr:if`,
`ssr:else-if` and `ssr:else`.

**Fix:** put it on the element right after the one with `ssr:if` or `ssr:else-if`

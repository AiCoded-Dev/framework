# E-GEN-007: invalid `ssr:for`

The value of `ssr:for` is not a loop. It names the item, optionally the index, and the list
to repeat the element for. It is also raised when an element has `ssr:for` together with
`ssr:if`, `ssr:else-if` or `ssr:else`, which would add the element twice.

**Fix:** write `item in list` or `i, item in list`

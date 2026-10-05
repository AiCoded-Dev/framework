# E-GEN-017: `ssr:bind` target cannot be written

An input's `ssr:bind` names a variable the page may not write. A bound input sends what the
viewer types to the server, which stores it in the variable, so the variable must be declared
in the same template with `reactive="true" client-writable="true"`. A variable of another page,
such as a parent layout, cannot be bound. The server passes every value to the data provider's
`Validate<Name>` method and stores only what it returns.

**Fix:** declare the variable in this template with `reactive="true" client-writable="true"`

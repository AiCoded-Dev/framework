# E-WEB-004: error status out of range

A hook returned `web.Error` with a status that is not an error status. The framework logs this
error and answers with 500.

**Fix:** pass a status from 400 to 599 to `web.Error`.

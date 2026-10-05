# E-CHK-004: a test failed

`aicoded check` runs the app's tests with `go test`, with the race detector when cgo works. The test the message names failed. The message carries the first 20 lines it printed, and the position is the first line of a test file that its output names. A message that names a package instead of a test means that the package's tests failed with no failed test, for example because `TestMain` panicked or the tests ran out of time.

**Fix:** fix the code or the test until go test passes.

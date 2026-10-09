# E-GATE-028: the tests stopped the checks

A test sent a signal to processes it did not start, killed them or waited on them, so the tests, or the go command that runs them, ended before they could report: the program that runs the tests in the delivery pipeline ended without their outcome, or `aicoded check` reported that a signal stopped the go command (E-CHK-008). The message says which. Tests run in a sandbox of their own, so they reach nothing outside it, but a test that stops the checks leaves its publish unchecked: the step fails with this problem. A test that runs out of time, memory or disk is E-GATE-012 instead.

```go
func TestCleanup(t *testing.T) {
	_ = syscall.Kill(-1, syscall.SIGKILL) // wrong: kills every process the test can see
}
```

**Fix:** do not signal, kill or wait on other processes in tests.

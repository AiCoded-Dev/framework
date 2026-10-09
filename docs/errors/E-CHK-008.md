# E-CHK-008: the go command was stopped

`aicoded check` runs the go command to build the app, vet it and run its tests, and `aicoded dev` runs it to build the app. A signal ended the go command before it finished, so what it printed is cut short and its outcome is unknown; the message names the command and the signal. Most often a test sent a signal to processes it did not start or killed them, such as every process it can see, which takes in the go command that runs the tests. Otherwise the computer ran out of memory and killed the go command, or someone stopped it. In the delivery pipeline, the tests step reports this as E-GATE-028.

```go
func TestCleanup(t *testing.T) {
	_ = syscall.Kill(-1, syscall.SIGKILL) // wrong: kills the go command that runs the test
}
```

**Fix:** do not signal, kill or wait on other processes in tests; when no test does, give the go command more memory and run the check again.

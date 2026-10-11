# E-REL-003: framework too old

The runner on the platform speaks the runner protocol of `aicoded.dev/framework` v0.4.0 and
later. The publish was built with an older version of the framework, or with a version the
platform cannot read, as the `go.mod` of its commit requires it. Nothing was released.

**Fix:** require `aicoded.dev/framework` v0.4.0 or later in go.mod, such as with `go get aicoded.dev/framework@latest`, run aicoded generate and aicoded check, commit, and publish again

# E-GATE-002: exclude line in go.mod

`go.mod` has an `exclude` line, which makes the go command skip a version of a module and choose another. The delivery pipeline builds an app from the versions its `require` lines name, and refuses the line. The problem is at the line.

**Fix:** remove the `exclude` line, and require the version you want.

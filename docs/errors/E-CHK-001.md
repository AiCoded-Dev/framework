# E-CHK-001: generated file out of date

A frozen run, such as `aicoded check --frozen`, generates the app in memory and compares the result with the app's folder without writing anything. A file `aicoded generate` writes differs from what it would write now, is missing, or should have been removed: a `route_gen.go` or `reactive_gen.ts`, a built asset, a client in `services/`, the snapshot `.aicoded/rpc.json`, the generated sections of the permission list `aicoded.yaml`, or the `dataprovider.go` stub of a new page. The message names the file. The security checks of the delivery pipeline will generate the app again and refuse it when the result differs, so generated files are never edited by hand.

**Fix:** run `aicoded generate` in the app's folder and keep the result; never edit generated files by hand.

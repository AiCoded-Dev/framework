# E-RPC-008: call not in the permission list

An app called a function of another app that the `services.calls` section of its permission list does not name, so the runner refused the call. `aicoded generate` writes that section from the functions of the generated clients (`services/<package>/client_gen.go`, where the package is the called app's name without dashes) that the app's code refers to. The call was made some other way, for example with `rpc.Call` directly, or the permission list is older than the code.

**Fix:** call other apps only through the functions of their generated client, and run `aicoded generate` (which `aicoded dev` runs at start) so that `services.calls` names them.

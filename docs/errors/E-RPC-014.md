# E-RPC-014: file in the folder of a generated client

`aicoded rpc add <app>` gives the calling app a client for the called app in `services/<pkg>/client_gen.go`, which `aicoded generate` writes. That folder belongs to the generator: when `aicoded generate` lists the functions the app calls in the services section of the permission list, it does not read `services/`. Any other file or folder there, most often Go code that uses the client, would call other apps without the permission list saying so, and the runner refuses such calls. The position names the entry.

**Fix:** move the file out of `services/<pkg>/`, for example into the package that uses it; that folder holds only the generated client.

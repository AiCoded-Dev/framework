# E-RPC-005: schema snapshot not valid

A schema snapshot is not as `aicoded` writes it. `.aicoded/rpc.json` pins the functions an app serves to other apps and the field numbers of their types; `.aicoded/services/<app>.json` is the copy an app holds of another app's snapshot. Each is JSON with the keys `app`, `methods` and `types`, where every field has a valid `number` used once in its type, never a retired one, and a `type` the snapshot knows. A hand-edited or damaged snapshot could send data under the wrong field numbers, so nothing is generated or started from it. A held copy must also sit in the file named after the app it holds, and name its types with exported Go names, because `aicoded generate` declares them in the client it writes from the copy.

**Fix:** never edit a snapshot by hand: restore `.aicoded/rpc.json` from the version history, or, for a copy in `.aicoded/services/`, run `aicoded rpc add <app>` again.

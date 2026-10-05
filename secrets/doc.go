// Package secrets reads secret values that the runner holds for the app, such as a key to sign
// links with. The permission list declares their names, never their values:
//
//	secrets: [link_key]
//
// [Get] fetches a secret when the app needs it; one the permission list does not declare is
// E-MAN-002. It returns a [Value], which shows as [REDACTED] wherever it is printed, logged or
// encoded. Only [Value.Reveal] returns the secret: call it where the value is used, and never
// log, store or show what it returns. In aicoded dev the values come from the developer's
// dev.yaml, and the logs and traces it keeps show a value of 4 bytes or more as [secret <name>].
//
// Read more in the guide docs/guides/settings-and-secrets.md, which aicoded explain and the MCP
// tool howto print as guides/settings-and-secrets.
package secrets

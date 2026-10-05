// Package config reads the app's settings: named text values that the permission list declares
// and the runner supplies, such as the address of a team or the day a report goes out:
//
//	settings: [team_address, report_day]
//
// [String] returns the value of a setting; one the permission list does not declare is
// E-MAN-001. The values arrive when the app starts and stay the same until it starts again. In
// aicoded dev they come from the developer's dev.yaml, and an app that lacks one does not start
// (E-DEV-003). A password or a key is never a setting: declare it as a secret and read it with
// package secrets.
//
// Read more in the guide docs/guides/settings-and-secrets.md, which aicoded explain and the MCP
// tool howto print as guides/settings-and-secrets.
package config

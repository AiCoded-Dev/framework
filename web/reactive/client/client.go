// Package client holds the browser side of live pages as TypeScript. aicoded generate bundles
// it into every app that has live pages.
package client

import "embed"

// Entry is the runtime's entry file in Files.
const Entry = "reactive.ts"

// Files holds the runtime's sources.
//
//go:embed *.ts *.css
var Files embed.FS

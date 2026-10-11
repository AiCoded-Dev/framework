// Notes is an app where staff add notes and see them live.
package main

import (
	"aicoded.dev/framework/app"

	"notes/deps"
	"notes/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler(&deps.Deps{})})
}

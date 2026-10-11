// Notes is an app to write notes and read them.
package main

import (
	"aicoded.dev/framework/app"

	"notes/deps"
	"notes/pages"
)

func main() {
	d := &deps.Deps{}
	app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
}

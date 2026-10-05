// Room maintenance lets hotel staff report problems in rooms, and hotel-ops follow them up.
package main

import (
	"aicoded.dev/framework/app"

	"room-maintenance/deps"
	"room-maintenance/pages"
)

func main() {
	d := deps.New()
	app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
}

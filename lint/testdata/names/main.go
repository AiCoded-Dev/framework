// Command names is an app whose code uses names that app code may not use.
package main

import (
	"aicoded.dev/framework/app"

	"names/bad"
	"names/pages"
)

func main() {
	bad.Keep()
	app.Main(app.Options{Handler: pages.NewHandler()})
}

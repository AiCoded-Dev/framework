// Command cookies is an app whose code sets and reads cookies, some of which app code may not.
package main

import (
	"aicoded.dev/framework/app"

	"cookies/bad"
	"cookies/good"
)

func main() {
	bad.Set(nil, "", "")
	good.Set(nil, "")
	app.Main(app.Options{})
}

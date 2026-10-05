// Command modules is an app that uses third-party modules.
package main

import (
	"fmt"

	"example.com/caps/run"
	"example.com/fine/greet"
	"example.com/link/clock"
)

func main() {
	fmt.Println(greet.Hello(), run.Name, clock.Now())
}

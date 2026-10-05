// Package run runs programs and fetches pages.
package run

import (
	"net/http"
	"os/exec"
)

// Name is the name of the package.
const Name = "run"

// Start runs the program name.
func Start(name string) error { return exec.Command(name).Run() }

var fetch = http.Get

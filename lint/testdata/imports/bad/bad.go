// Package bad imports what app code may not.
package bad

import (
	_ "net"
	_ "os"
	_ "reflect"

	_ "aicoded.dev/framework/manifest"
	_ "aicoded.dev/framework/runnerproto"
	_ "google.golang.org/protobuf/proto"
)

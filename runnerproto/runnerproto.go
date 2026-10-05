// Package runnerproto defines the protocol between an app and the runner that hosts it.
// Framework packages and runners import it; apps do not.
package runnerproto

import "time"

// ProtocolVersion is the version of the app–runner protocol this framework speaks.
const ProtocolVersion uint32 = 3

// MaxMessageBytes bounds one message on the runner socket: a runner refuses a larger request and
// an app a larger response.
const MaxMessageBytes = 16 << 20

// EnvRunnerDir names the only environment variable an app gets: the directory holding
// the runner's sockets.
const EnvRunnerDir = "AICODED_RUNNER_DIR"

// Socket file names inside the runner directory.
const (
	RunnerSocket = "runner.sock"
	AppSocket    = "app.sock"
	// MySQLSocket exists only when the app declares sqldb.
	MySQLSocket = "mysql.sock"
)

// Deadlines of calls between apps.
const (
	// DefaultCallDeadline applies to a call whose context has no deadline.
	DefaultCallDeadline = 10 * time.Second
	// MaxCallDeadline caps the deadline of every call.
	MaxCallDeadline = 30 * time.Second
)

// MaxForwardedTokenAge is how long after issuing a viewer token the runner still accepts it
// back from the app, to call another app as that viewer: the lifetime of a live page connection.
const MaxForwardedTokenAge = 10 * time.Minute

// The runner marks every refusal of a call between apps that it makes itself with the error
// metadata OriginHeader: OriginRunner. An error the called app returns never carries the mark,
// so an app reads an error code only from a marked error.
const (
	OriginHeader = "Aicoded-Origin"
	OriginRunner = "runner"
)

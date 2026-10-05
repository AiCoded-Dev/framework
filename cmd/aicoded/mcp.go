package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"aicoded.dev/framework/cmd/aicoded/internal/mcp"
	"aicoded.dev/framework/cmd/aicoded/internal/scaffold"
)

// runMCP serves the local MCP server over stdin and stdout for the apps in the folder args
// names, or the current one. Only the protocol reaches stdout: while it runs, os.Stdout is
// stderr.
func runMCP(args []string, stdout, stderr io.Writer) int {
	saved := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = saved }()

	rest, code, ok := parseArgs(newFlags("mcp", stderr), args, 0, 1)
	if !ok {
		return code
	}
	// Without a framework, app_create answers E-CLI-003; the other tools need none.
	fw, _ := scaffold.Detect(frameworkDir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mcp.Version = version
	t := &mcpsdk.IOTransport{Reader: os.Stdin, Writer: nopCloser{stdout}}
	if err := mcp.Serve(ctx, dirArg(rest), t, stderr, fw); err != nil {
		return fail(stderr, err)
	}
	return exitOK
}

// nopCloser leaves the writer open when the MCP session ends.
type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

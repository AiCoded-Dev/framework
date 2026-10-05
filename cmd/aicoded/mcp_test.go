package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
)

const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

// stdin makes the read end of a new pipe os.Stdin until the test ends, and returns its write end.
func stdin(t *testing.T) *os.File {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	saved := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = saved
		_ = r.Close()
		_ = w.Close()
	})
	return w
}

func TestRunMCPClosedInput(t *testing.T) {
	testhome.Set(t)
	require.NoError(t, stdin(t).Close())
	stdout := os.Stdout
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run([]string{"mcp", t.TempDir()}, &out, &errOut), errOut.String())
	assert.Empty(t, out.String())
	assert.Same(t, stdout, os.Stdout, "os.Stdout is restored")
}

func TestRunMCPWritesOnlyTheProtocol(t *testing.T) {
	testhome.Set(t)
	in, dir := stdin(t), t.TempDir()
	pr, pw := io.Pipe()
	var errOut bytes.Buffer
	code := make(chan int, 1)
	go func() {
		code <- run([]string{"mcp", dir}, pw, &errOut)
		_ = pw.Close()
	}()

	_, err := in.WriteString(initialize + "\n")
	require.NoError(t, err)
	out := bufio.NewReader(pr)
	line, err := out.ReadBytes('\n')
	require.NoError(t, err)
	var msg struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ServerInfo struct{ Name string } `json:"serverInfo"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(line, &msg))
	assert.Equal(t, "2.0", msg.JSONRPC)
	assert.Equal(t, 1, msg.ID)
	assert.Equal(t, "aicoded", msg.Result.ServerInfo.Name)

	rest := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(out)
		rest <- data
	}()
	require.NoError(t, in.Close())
	assert.Equal(t, 0, <-code, errOut.String())
	assert.Empty(t, <-rest, "nothing but the answer reached stdout")
}

func TestRunMCPUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"mcp", "a", "b"}, &out, &errOut))
	assert.Empty(t, out.String())
}

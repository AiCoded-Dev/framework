package mcp

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the commands that add aicoded mcp to an AI assistant.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data writes the setup of each assistant for this aicoded and the workspace's root.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	st, err := p.d.Backend.Status(ctx)
	if err != nil {
		return err
	}
	args := " -- " + shellQuote(p.d.Executable) + " mcp " + shellQuote(st.Root)
	data.Root = st.Root
	data.Claude = "claude mcp add aicoded" + args
	data.Codex = "codex mcp add aicoded" + args
	data.Cursor, err = cursorConfig(p.d.Executable, st.Root)
	return err
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell, unless it holds only characters that need no quotes.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// cursorConfig returns the .cursor/mcp.json that runs aicoded mcp at exe for root.
func cursorConfig(exe, root string) (string, error) {
	type server struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(map[string]map[string]server{"mcpServers": {"aicoded": {Command: exe, Args: []string{"mcp", root}}}})
	return strings.TrimSuffix(b.String(), "\n"), err
}

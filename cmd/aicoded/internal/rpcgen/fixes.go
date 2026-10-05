package rpcgen

import (
	"fmt"

	"aicoded.dev/framework/internal/errs"
)

// fixes holds the fix line of every code the package raises.
var fixes = map[string]string{
	"E-RPC-001": "add //ssr:access caller=<app> role=<role> on the line right above the function, with apps=true in place of or next to role=, or unexport the function",
	"E-RPC-002": "write one //ssr:access caller=<app>[,<app>] line, with no space after //, right above an exported function, with role=<role>[|<role>] or role=*, apps=true, or both",
	"E-RPC-003": "write func Name(ctx context.Context, in NameIn) (NameOut, error) in package rpc with exported struct types of the package, and rename anything named like a Go type or like Server, aicodedAppend or aicodedDecode",
	"E-RPC-004": "use bool, int32, int64, uint32, uint64, float32, float64, string, []byte, time.Time or an exported struct of package rpc, a slice of one of these, or a pointer to one that is not a slice",
	"E-RPC-005": "never edit a snapshot by hand: restore .aicoded/rpc.json from the version history, or run aicoded rpc add <app> again for a copy in .aicoded/services/",
	"E-RPC-006": "rename the called app in its aicoded.yaml, so that its name without dashes is not a Go keyword, a predeclared name, main or init, and differs from the other apps this app calls; then remove its old .aicoded/services/<app>.json here, if there is one, and run aicoded rpc add <new name>",
}

// fix returns the fix line of code, or "" for a code the package does not raise.
func fix(code string) string { return fixes[code] }

// diag returns a coded error at pos with the fix line of code. It panics when code has no fix
// line, because that is a bug in the generator.
func diag(pos, code, format string, args ...any) *errs.Error {
	f := fix(code)
	if f == "" {
		panic("rpcgen: no fix line for " + code)
	}
	return errs.At(pos, code, fmt.Sprintf(format, args...), f)
}

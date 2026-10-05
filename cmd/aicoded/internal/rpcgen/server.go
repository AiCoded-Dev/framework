package rpcgen

import (
	"strconv"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
)

// Import paths of the packages generated code uses. The framework's calls runtime is imported
// as frameworkrpc, because the app's own package is named rpc too.
const (
	frameworkRPC = "aicoded.dev/framework/rpc"
	wirePkg      = "aicoded.dev/framework/rpc/wire"
	protowirePkg = "google.golang.org/protobuf/encoding/protowire"
)

// Server returns rpc/server_gen.go of the package p, whose types s numbers: the codec of the
// types, and the function Server, which serves every function of p with the callers, role rule
// and apps flag of its //ssr:access line. A served function decodes its input, runs, and encodes
// its output.
func Server(p Package, s rpcschema.Schema) ([]byte, error) {
	b := gobuf.New()
	b.WriteString(gobuf.Header)
	b.WriteStringLn("package rpc")
	b.WriteStringLn("")
	b.WriteStringLn("import (")
	b.WriteQuotedString("context", "\n\n")
	b.WriteQuotedString(protowirePkg, "\n\n")
	b.WriteString("frameworkrpc ")
	b.WriteQuotedString(frameworkRPC, "\n")
	b.WriteQuotedString(wirePkg, "\n")
	b.WriteStringLn(")")
	b.WriteStringLn("")
	b.WriteStringLn("// Server returns the functions of this package as the app serves them to other apps.")
	b.WriteStringLn("func Server() *frameworkrpc.Server {")
	b.WriteStringLn("return frameworkrpc.NewServer(")
	for _, f := range p.Funcs {
		b.WriteStringLn("frameworkrpc.Method{")
		b.WriteString("Name: ")
		b.WriteQuotedString(f.Name, ",\n")
		b.WriteStringLn("Callers: " + stringSlice(f.Access.Callers) + ",")
		if len(f.Access.Require) > 0 {
			b.WriteStringLn("Require: " + stringSlice(f.Access.Require) + ",")
		}
		if f.Access.Apps {
			b.WriteStringLn("Apps: true,")
		}
		b.WriteStringLn("Call: func(ctx context.Context, in []byte) ([]byte, error) {")
		b.WriteStringLn("var x " + f.In)
		b.WriteStringLn("if err := wire.Decode(in, x.aicodedDecode); err != nil {")
		b.WriteStringLn("return nil, frameworkrpc.DecodeError(err)")
		b.WriteStringLn("}")
		b.WriteStringLn("out, err := " + f.Name + "(ctx, x)")
		b.WriteStringLn("if err != nil {")
		b.WriteStringLn("return nil, err")
		b.WriteStringLn("}")
		b.WriteStringLn("return out.aicodedAppend(nil), nil")
		b.WriteStringLn("},")
		b.WriteStringLn("},")
	}
	b.WriteStringLn(")")
	b.WriteStringLn("}")
	b.WriteStringLn("")
	b.WriteString(string(Codec(s, false)))
	return b.Formatted()
}

// stringSlice returns the Go literal of ss.
func stringSlice(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = strconv.Quote(s)
	}
	return "[]string{" + strings.Join(q, ", ") + "}"
}

package rpcgen

import (
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
)

// heldFile returns the position of the snapshot of app that a calling app holds.
func heldFile(app string) string { return ".aicoded/services/" + app + ".json:1" }

// PackageName returns the name of the client package of app: the app's name without its dashes.
// It refuses, with E-RPC-006 and no position, a name that is not an app name, and one whose
// package name Go reserves: a keyword, a predeclared identifier, main or init.
func PackageName(app string) (string, error) { return packageName("", app) }

// packageName is PackageName with its errors at pos.
func packageName(pos, app string) (string, error) {
	if !manifest.ValidApp(app) {
		return "", diag(pos, "E-RPC-006", "%q is not an app name", app)
	}
	name := strings.ReplaceAll(app, "-", "")
	if token.IsKeyword(name) || types.Universe.Lookup(name) != nil || name == "main" || name == "init" {
		return "", diag(pos, "E-RPC-006", "the client of the app %s would be the package %s, a name Go reserves", app, name)
	}
	return name, nil
}

// PackageNames returns the client package name of every app in apps, whose snapshots a calling
// app holds, by app. It refuses, with E-RPC-006 at the held snapshot, what PackageName refuses
// and two apps whose clients would have the same name.
func PackageNames(apps []string) (map[string]string, error) {
	out := map[string]string{}
	byName := map[string]string{}
	for _, app := range slices.Sorted(slices.Values(apps)) {
		name, err := packageName(heldFile(app), app)
		if err != nil {
			return nil, err
		}
		if other, ok := byName[name]; ok {
			return nil, diag(heldFile(app), "E-RPC-006", "the clients of the apps %s and %s would both be the package %s", other, app, name)
		}
		byName[name] = app
		out[app] = name
	}
	return out, nil
}

// AddedPackageName returns the client package name of app for a calling app that holds the
// snapshots of the apps in held, before it holds that of app. It refuses, with E-RPC-006, what
// PackageNames refuses in held, and, with no position, what PackageName refuses and a package
// name that the client of another app in held has.
func AddedPackageName(app string, held []string) (string, error) {
	names, err := PackageNames(held)
	if err != nil {
		return "", err
	}
	name, err := PackageName(app)
	if err != nil {
		return "", err
	}
	for _, other := range slices.Sorted(maps.Keys(names)) {
		if other != app && names[other] == name {
			return "", diag("", "E-RPC-006", "the client of %s would be the package %s, which is the client of %s, an app this app calls already", app, name, other)
		}
	}
	return name, nil
}

// Client returns services/<package>/client_gen.go for app, whose snapshot, as rpcschema.Parse
// read it, a calling app holds as s: the types of s with their codec, and for every function of
// app a function of the same name that calls it through the runner. It refuses, with E-RPC-005, a
// type name that is not exported, and what PackageName refuses.
func Client(app string, s rpcschema.Schema) ([]byte, error) {
	pkg, err := packageName(heldFile(app), app)
	if err != nil {
		return nil, err
	}
	if err := checkHeld(app, s); err != nil {
		return nil, err
	}
	usesTime := false
	for _, t := range s.Types {
		for _, f := range t.Fields {
			usesTime = usesTime || strings.TrimLeft(f.Type, "[]*") == "time"
		}
	}
	b := gobuf.New()
	b.WriteString(gobuf.Header)
	b.WriteStringLn("// Package " + pkg + " calls the functions of the app " + app + " through the runner.")
	b.WriteStringLn("package " + pkg)
	b.WriteStringLn("")
	b.WriteStringLn("import (")
	if len(s.Methods) > 0 {
		b.WriteQuotedString("context", "\n")
	}
	if usesTime {
		b.WriteQuotedString("time", "\n")
	}
	b.WriteStringLn("")
	if len(s.Types) > 0 {
		b.WriteQuotedString(protowirePkg, "\n\n")
	}
	if len(s.Methods) > 0 {
		b.WriteString("frameworkrpc ")
		b.WriteQuotedString(frameworkRPC, "\n")
	}
	if len(s.Types) > 0 {
		b.WriteQuotedString(wirePkg, "\n")
	}
	b.WriteStringLn(")")
	b.WriteStringLn("")
	for _, name := range slices.Sorted(maps.Keys(s.Methods)) {
		m := s.Methods[name]
		b.WriteStringLn("// " + name + " calls " + name + " of the app " + app + " as the viewer of ctx, or as this app when ctx")
		b.WriteStringLn("// carries no viewer.")
		b.WriteStringLn("func " + name + "(ctx context.Context, in " + m.In + ") (" + m.Out + ", error) {")
		b.WriteString("b, err := frameworkrpc.Call(ctx, ")
		b.WriteQuotedString(app, ", ")
		b.WriteQuotedString(name, ", in.aicodedAppend(nil))\n")
		b.WriteStringLn("if err != nil {")
		b.WriteStringLn("return " + m.Out + "{}, err")
		b.WriteStringLn("}")
		b.WriteStringLn("var out " + m.Out)
		b.WriteStringLn("if err := wire.Decode(b, out.aicodedDecode); err != nil {")
		b.WriteStringLn("return " + m.Out + "{}, err")
		b.WriteStringLn("}")
		b.WriteStringLn("return out, nil")
		b.WriteStringLn("}")
		b.WriteStringLn("")
	}
	b.WriteString(string(Codec(s, true)))
	return b.Formatted()
}

// checkHeld refuses, with E-RPC-005, a held snapshot of app with a type whose name is not
// exported. rpcschema.Parse has checked the other names; the client declares the types next to
// its lower-case imports.
func checkHeld(app string, s rpcschema.Schema) error {
	for name := range s.Types {
		if !token.IsExported(name) {
			return diag(heldFile(app), "E-RPC-005", "the type name %q is not an exported Go name", name)
		}
	}
	return nil
}

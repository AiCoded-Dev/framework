package lint

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Framework is the module path of the framework.
const Framework = "aicoded.dev/framework"

// blocks are the framework's packages for apps, the building blocks, by path in the framework.
var blocks = []string{
	"app", "auth", "config", "filestore", "mailer", "rpc", "secrets", "sqldb", "telemetry",
	"web", "web/form", "web/reactive",
}

// std are the standard packages app code may import.
var std = []string{
	"bufio", "bytes", "cmp", "container/heap", "container/list", "context",
	"crypto/hmac", "crypto/rand", "crypto/sha256", "crypto/sha512", "crypto/subtle",
	"database/sql", "database/sql/driver",
	"encoding", "encoding/base32", "encoding/base64", "encoding/binary", "encoding/csv", "encoding/hex",
	"encoding/json", "encoding/xml",
	"errors", "fmt", "hash", "hash/crc32", "hash/fnv", "html", "io", "io/fs", "iter", "log/slog", "maps",
	"math", "math/big", "math/bits", "math/rand/v2",
	"net/http", "net/mail", "net/url",
	"path", "regexp", "slices", "sort", "strconv", "strings", "sync", "sync/atomic", "time",
	"unicode", "unicode/utf16", "unicode/utf8",
}

// generatedOnly are the packages that only generated files may import.
var generatedOnly = []string{
	"embed",
	"google.golang.org/protobuf/encoding/protowire",
	Framework + "/rpc/wire",
	Framework + "/web/render",
}

var (
	// allowed holds the import paths of blocks and std.
	allowed = setOf(append(prefixed(Framework+"/", blocks), std...))
	// forGenerated holds generatedOnly.
	forGenerated = setOf(generatedOnly)
)

// Blocks returns the import paths of the framework's packages that app code may import.
func Blocks() []string {
	return prefixed(Framework+"/", blocks)
}

// imports reports, with E-LINT-001, every import of a package that is not one of the app's own,
// a building block, an allowed standard package or a package of a third-party module. A
// generated file may also import the packages of generatedOnly. A package whose path is the
// framework's or under it must belong to the framework module. A package of the app that ./...
// leaves out, in testdata, in a folder whose name starts with _ or ., or in a folder that go.mod
// ignores, is not one of the app's own, since the checks do not read it. A third-party module is
// declared in modules: or reported by E-LINT-002, and its packages are checked by E-LINT-003.
func (a *app) imports(p *analysis.Pass) {
	for _, f := range p.Files {
		generated := a.isGenerated(p, f)
		for _, spec := range f.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			switch {
			case a.notFramework(path):
				diagnose(p, spec.Pos(), "E-LINT-001", fmt.Sprintf("app code may not import %q, which is not a package of the framework module", path))
			case a.own[path] || allowed[path] || generated && forGenerated[path]:
			case forGenerated[path]:
				diagnose(p, spec.Pos(), "E-LINT-001", fmt.Sprintf("only generated files may import %q", path))
			case a.unread(path):
				diagnose(p, spec.Pos(), "E-LINT-001", fmt.Sprintf("app code may not import %q, a package of the app that the checks do not read: "+
					"move it out of testdata, folders whose names start with _ or ., and folders that go.mod ignores", path))
			case a.thirdParty(path):
			default:
				diagnose(p, spec.Pos(), "E-LINT-001", fmt.Sprintf("app code may not import %q", path))
			}
		}
	}
}

// notFramework reports whether path is the framework's path or under it, but its package does
// not belong to the framework module.
func (a *app) notFramework(path string) bool {
	if path != Framework && !strings.HasPrefix(path, Framework+"/") {
		return false
	}
	m := a.module[path]
	return m == nil || m.Path != Framework
}

// unread reports whether path is a package of the app's own module that is not one of the
// packages of ./..., which the checks read.
func (a *app) unread(path string) bool {
	m := a.module[path]
	return m != nil && m.Main && !a.own[path]
}

// prefixed returns the names, each with prefix in front.
func prefixed(prefix string, names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, prefix+n)
	}
	return out
}

// setOf returns the set of names.
func setOf(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

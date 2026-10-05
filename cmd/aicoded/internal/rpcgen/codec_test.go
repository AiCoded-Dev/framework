package rpcgen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
)

// modulePath names the modules the tests build. It lies inside the framework's path, so their
// tests may use the framework's internal packages to stand in for the runner.
const modulePath = "aicoded.dev/framework/rpcgentest"

// testModule copies testdata/<name> into a temp dir, adds files by path, and makes the dir a Go
// module that requires the framework, replaced by this checkout, and its dependencies.
func testModule(t *testing.T, name string, files map[string][]byte) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	gosum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", name))))
	mod := strings.Replace(string(gomod), "module aicoded.dev/framework\n", "module "+modulePath+"\n", 1) +
		"\nrequire aicoded.dev/framework v0.0.0-00010101000000-000000000000\n\nreplace aicoded.dev/framework => " + root + "\n"
	write(t, dir, "go.mod", mod)
	write(t, dir, "go.sum", string(gosum))
	for rel, data := range files {
		write(t, dir, rel, string(data))
	}
	return dir
}

// goTest runs the tests of the module in dir.
func goTest(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "test", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func field(n int, typ string) rpcschema.Field { return rpcschema.Field{Number: n, Type: typ} }

// allKinds has a field of every kind the codec carries. Child numbers its fields against the
// order they are declared in.
var allKinds = rpcschema.Schema{Types: map[string]rpcschema.Type{
	"All": {Fields: map[string]rpcschema.Field{
		"Bool": field(1, "bool"), "I32": field(2, "int32"), "I64": field(3, "int64"), "U32": field(4, "uint32"),
		"U64": field(5, "uint64"), "F32": field(6, "float32"), "F64": field(7, "float64"), "Str": field(8, "string"),
		"Raw": field(9, "bytes"), "When": field(10, "time"), "Child": field(11, "Child"), "PBool": field(12, "*bool"),
		"PI64": field(13, "*int64"), "PStr": field(14, "*string"), "PWhen": field(15, "*time"), "PChild": field(16, "*Child"),
		"Bools": field(17, "[]bool"), "I32s": field(18, "[]int32"), "I64s": field(19, "[]int64"), "U32s": field(20, "[]uint32"),
		"U64s": field(21, "[]uint64"), "F32s": field(22, "[]float32"), "F64s": field(23, "[]float64"),
		"Strs": field(24, "[]string"), "Raws": field(25, "[]bytes"), "Whens": field(26, "[]time"),
		"Children": field(27, "[]Child"), "Empty": field(28, "Empty"),
	}},
	"Child": {Fields: map[string]rpcschema.Field{"Next": field(1, "*Child"), "Name": field(2, "string")}},
	"Empty": {Fields: map[string]rpcschema.Field{}},
}}

// TestCodec builds the codec of allKinds, with its types declared, and runs the tests in
// testdata/codec against it: round trips, zero values, pointers, unknown fields, packed repeated
// scalars and malformed input.
func TestCodec(t *testing.T) {
	src := "package codec\n\nimport (\n\t\"time\"\n\n\t\"google.golang.org/protobuf/encoding/protowire\"\n\n\t\"aicoded.dev/framework/rpc/wire\"\n)\n\n" +
		string(Codec(allKinds, true))
	goTest(t, testModule(t, "codec", map[string][]byte{"codec/codec_gen.go": []byte(src)}))
}

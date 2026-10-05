package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageCallsCompile(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	goVet(t, dir)

	code, err := os.ReadFile(filepath.Join(dir, "pages/tools/route_gen.go"))
	require.NoError(t, err)
	golden(t, code, "tools_route_gen.go.golden")
	assert.Contains(t, string(code), "CallAdd(ctx context.Context, r *web.Request, in AddIn) (AddOut, error)")
	assert.Contains(t, string(code), "web.DecodeArgs(args, &in)")
	assert.Contains(t, string(code), `web.Error(http.StatusNotFound, "This call is not on this page.")`)

	ts, err := os.ReadFile(filepath.Join(dir, "pages/tools/reactive_gen.ts"))
	require.NoError(t, err)
	for _, want := range []string{
		"add: { in: AddIn; out: AddOut };",
		"export type AddIn = {",
		"  a: number;",
		"  note?: string;",
		"export type AddOut = {",
		"  sum: number;",
		".aicodedReactive.route<ReadVars, WriteVars, Calls>(",
	} {
		assert.Contains(t, string(ts), want)
	}
	assert.NotContains(t, string(ts), "Secret")
	assert.NotContains(t, string(ts), "secret")

	handler, err := os.ReadFile(filepath.Join(dir, "pages/handler_gen.go"))
	require.NoError(t, err)
	golden(t, handler, "handler_gen.go.golden")
}

func TestCallCode(t *testing.T) {
	code := routeCode(t, `<ssr:access role="*"/><ssr:assets/><ssr:call name="add" in="AddIn" out="*AddOut"/><ssr:call name="ping" in="struct{}" out="string"/>`)
	assert.Contains(t, code, "\tCallAdd(ctx context.Context, r *web.Request, in AddIn) (*AddOut, error)\n")
	assert.Contains(t, code, "\tCallPing(ctx context.Context, r *web.Request, in struct{}) (string, error)\n")
	assert.Equal(t, `func (s *state) Call(ctx context.Context, r *web.Request, name string, args json.RawMessage) (any, error) {
	switch name {
	case "add":
		var in AddIn
		if err := web.DecodeArgs(args, &in); err != nil {
			return nil, err
		}
		return s.dp.CallAdd(ctx, r, in)
	case "ping":
		var in struct{}
		if err := web.DecodeArgs(args, &in); err != nil {
			return nil, err
		}
		return s.dp.CallPing(ctx, r, in)
	}
	return nil, web.Error(http.StatusNotFound, "This call is not on this page.")
}
`, body(t, code, "func (s *state) Call("))
	assert.Equal(t, 1, strings.Count(code, "RouteKey()"))
	assert.NotContains(t, code, "Snapshot", "calls alone add no live-value code")
	assert.NotContains(t, code, "sync")

	code = liveCode(t, `<ssr:var name="n" type="int" reactive="true"/><ssr:call name="add" in="int" out="int"/>`)
	assert.Equal(t, 1, strings.Count(code, "func (s *state) RouteKey() string { return routeKey }"), "one RouteKey for live values and calls")
	assert.Contains(t, code, "func (s *state) Call(")
	assert.Contains(t, code, "func (s *state) Snapshot(")

	code = liveCode(t, `<p>none</p>`)
	assert.NotContains(t, code, "Call")
	assert.NotContains(t, code, "RouteKey()")
	assert.NotContains(t, code, "encoding/json")
}

func TestCallStub(t *testing.T) {
	g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:assets/><ssr:call name="add" in="AddIn" out="AddOut"/>`)})
	assert.Contains(t, string(g.stubs["pages/dataprovider.go"]), "// CallAdd answers the page's call add. It refuses every call until it checks that the viewer\n"+
		"// may make it and what the page sent.\n"+
		"func (p *DP) CallAdd(ctx context.Context, r *web.Request, in AddIn) (AddOut, error) {\n"+
		"\tvar out AddOut\n\treturn out, web.Forbidden()\n}")
}

func TestLive(t *testing.T) {
	g := built(t, map[string]string{
		"pages/index.html":       doc(`<ssr:access role="*"/><ssr:assets/><ssr:content/>`),
		"pages/calls/index.html": `<ssr:call name="add" in="int" out="int"/>`,
		"pages/vars/index.html":  `<ssr:var name="n" type="int" reactive="true"/>`,
	})
	assert.False(t, g.live("/"))
	assert.True(t, g.live("/calls"))
	assert.True(t, g.live("/vars"))
	assert.False(t, g.live("/gone"))
	assert.NotContains(t, g.files, "pages/reactive_gen.ts")

	ts := string(g.files["pages/calls/reactive_gen.ts"])
	assert.Contains(t, ts, "export type ReadVars = {};\n\nexport type WriteVars = {};\n\nexport type Calls = {\n  add: { in: number; out: number };\n};\n")
	assert.NotContains(t, string(g.files["pages/vars/reactive_gen.ts"]), "add")
	assert.Contains(t, string(g.files["pages/vars/reactive_gen.ts"]), "export type Calls = Record<string, never>;\n")
}

// callTypes is a route package whose types cover how encoding/json writes and reads a value.
const callTypes = `package pages

import (
	"encoding/json"
	stdtime "time"
)

type In struct {
	ID      ID              ` + "`json:\"id\"`" + `
	Name    string
	Tagged  string          ` + "`json:\"tagged,omitempty\"`" + `
	Zero    int             ` + "`json:\",omitzero\"`" + `
	Skipped string          ` + "`json:\"-\"`" + `
	Dash    string          ` + "`json:\"-,\"`" + `
	hidden  string
	When    stdtime.Time    ` + "`json:\"when\"`" + `
	Maybe   *stdtime.Time   ` + "`json:\"maybe\"`" + `
	Wait    stdtime.Duration ` + "`json:\"wait\"`" + `
	Raw     json.RawMessage ` + "`json:\"raw\"`" + `
	Data    []byte          ` + "`json:\"data\"`" + `
	Grid    [2][3]int       ` + "`json:\"grid\"`" + `
	Tags    []string        ` + "`json:\"tags\"`" + `
	Counts  map[string]*int ` + "`json:\"counts\"`" + `
	Count   int64           ` + "`json:\"count,string\"`" + `
	Kebab   bool            ` + "`json:\"kebab-case\"`" + `
	Quote   int             ` + "`json:\"it's\"`" + `
	Any     any             ` + "`json:\"any\"`" + `
	Fn      func()          ` + "`json:\"fn\"`" + `
	Inline  struct {
		X int ` + "`json:\"x\"`" + `
		Y int ` + "`json:\"y,omitempty\"`" + `
	} ` + "`json:\"inline\"`" + `
	Base
	*Extra
	meta
	score
	Named Base      ` + "`json:\"named\"`" + `
	Tree  *Tree     ` + "`json:\"tree\"`" + `
	Stamp Stamp     ` + "`json:\"stamp\"`" + `
	Code  Code      ` + "`json:\"code\"`" + `
	Page  Page[int] ` + "`json:\"page\"`" + `
	Other Missing   ` + "`json:\"other\"`" + `
}

type ID int

type Base struct {
	Owner string ` + "`json:\"owner\"`" + `
	Name  int
}

type Extra struct {
	Note string ` + "`json:\"note\"`" + `
}

type meta struct {
	Rev int ` + "`json:\"rev\"`" + `
}

type score int

type Tree struct {
	Children []Tree ` + "`json:\"children\"`" + `
}

type Stamp struct{}

func (Stamp) MarshalJSON() ([]byte, error) { return nil, nil }

type Code struct{}

func (*Code) UnmarshalText([]byte) error { return nil }

type Level struct{ V int }

func (Level) MarshalText() ([]byte, error) { return nil, nil }

func (*Level) UnmarshalText([]byte) error { return nil }

type Stamped struct {
	Stamp
	X int
}

type Coded struct {
	*Code
	X int
}

type Leveled struct {
	Level
	X int
}

type Inner struct{ Stamp }

type Outer struct {
	Inner ` + "`json:\"inner\"`" + `
	X     int
}

type Code2 Code

type Shapes struct {
	Stamped Stamped ` + "`json:\"stamped\"`" + `
	Coded   Coded   ` + "`json:\"coded\"`" + `
	Leveled Leveled ` + "`json:\"leveled\"`" + `
	Level   Level   ` + "`json:\"level\"`" + `
	Outer   Outer   ` + "`json:\"outer\"`" + `
	Plain   Code2   ` + "`json:\"plain\"`" + `
	Wrapped Wrapped ` + "`json:\"wrapped\"`" + `
}

type Page[T any] struct{ Items []T }

type A struct{ X int }
type B struct{ X int }
type C struct {
	Y string ` + "`json:\"Y\"`" + `
}
type D struct{ Y int }

type Both struct {
	A
	B
	C
	D
}

type sumArgs struct {
	A int ` + "`json:\"a\"`" + `
}

type class int

type Route struct {
	Path string ` + "`json:\"path\"`" + `
	Next *Route ` + "`json:\"next\"`" + `
}

type Alias = Tree

type Wrapped struct {
	json.Number
	Label string ` + "`json:\"label\"`" + `
}
`

func TestCallTS(t *testing.T) {
	g := built(t, map[string]string{
		"pages/index.html": doc(`<ssr:access role="*"/><ssr:assets/><ssr:var name="b" type="Base" reactive="true"/>
<ssr:call name="save" in="In" out="*Tree"/><ssr:call name="ping" in="struct{}" out="[]ID"/>
<ssr:call name="both" in="Both" out="Wrapped"/><ssr:call name="sum" in="sumArgs" out="class"/>
<ssr:call name="walk" in="Route" out="Alias"/><ssr:call name="shapes" in="Shapes" out="Code"/>`),
		"pages/types.go":      callTypes,
		"pages/types_test.go": "package pages\n\ntype Hidden struct{ X int }\n",
	})
	ts := string(g.files["pages/reactive_gen.ts"])
	for _, want := range []string{
		`export type In = {
  id: ID;
  Name: string;
  tagged?: string;
  Zero?: number;
  "-": string;
  when: string;
  maybe: string | null;
  wait: number;
  raw: unknown;
  data: string | null;
  grid: number[][];
  tags: string[] | null;
  counts: Record<string, number | null> | null;
  count: string;
  "kebab-case": boolean;
  Quote: number;
  any: unknown;
  fn: unknown;
  inline: { x: number; y?: number };
  owner: string;
  note?: string;
  rev: number;
  named: Base;
  tree: Tree | null;
  stamp: unknown;
  code: unknown;
  page: unknown;
  other: unknown;
};`,
		"export type ID = number;",
		"export type Base = {\n  owner: string;\n  Name: number;\n};",
		"export type Tree = {\n  children: Tree[] | null;\n};",
		"export type Both = {\n  Y: string;\n};",
		"export type Shapes = {\n  stamped: unknown;\n  coded: unknown;\n  leveled: string;\n  level: string;\n  outer: unknown;\n  plain: Code2;\n  wrapped: unknown;\n};",
		"export type Code2 = Record<string, never>;",
		"export type Calls = {\n  both: { in: Both; out: unknown };\n  ping: { in: Record<string, never>; out: ID[] | null };\n  save: { in: In; out: Tree | null };\n" +
			"  shapes: { in: Shapes; out: unknown };\n  sum: { in: { a: number }; out: number };\n" +
			"  // type \"Route\" is written in place, because reactive_gen.ts cannot declare that name\n" +
			"  // cyclic type \"Route\" substituted with unknown in TypeScript interface\n" +
			"  walk: { in: { path: string; next: unknown }; out: Tree };\n};",
		"export type ReadVars = {\n  b: Base;\n};",
	} {
		assert.Contains(t, ts, want+"\n")
	}
	assert.Equal(t, 1, strings.Count(ts, "type Base"), "a call's type and a live variable's type are declared once")
	for _, never := range []string{"Skipped", "hidden", "score", "Hidden", "Extra", "export type Stamp", "export type meta",
		"export type sumArgs", "export type class", "export type Route =", "export type Alias", "export type Coded",
		"export type Leveled", "export type Level", "export type Outer", "export type Wrapped", "export type Code "} {
		assert.NotContains(t, ts, never)
	}
}

func TestLiveAndCallTypesAgree(t *testing.T) {
	ts := string(built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:assets/><ssr:var name="tags" type="[]string" reactive="true"/>
<ssr:var name="blobs" type="map[int][]byte" reactive="true"/><ssr:call name="get" in="[]string" out="map[int][]byte"/>`)}).files["pages/reactive_gen.ts"])
	assert.Contains(t, ts, "export type ReadVars = {\n  blobs: Record<string, string | null> | null;\n  tags: string[] | null;\n};\n")
	assert.Contains(t, ts, "  get: { in: string[] | null; out: Record<string, string | null> | null };\n")
}

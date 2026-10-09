package gotool

import (
	"fmt"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

// ran is a run of the go command in /work/app that printed stdout and stderr.
func ran(cmd, stdout, stderr string, failed bool) result {
	return result{cmd: cmd, dir: "/work/app", stdout: []byte(stdout), stderr: []byte(stderr), failed: failed}
}

// What a run that a signal ended printed is cut short: its one problem is E-CHK-008, whatever
// else it printed.
func TestStoppedRunProblems(t *testing.T) {
	failure := `{"Action":"fail","Package":"app","Test":"TestX"}` + "\n"
	for _, c := range []struct {
		cmd      string
		problems func(result) []*errs.Error
	}{
		{"go build", buildProblems},
		{"go vet", vetProblems},
		{"go test", func(r result) []*errs.Error { return testProblems(r, "app") }},
	} {
		r := ran(c.cmd, failure, "main.go:3:1: undefined: x\n", true)
		r.signal = syscall.SIGTERM
		assert.Equal(t, []brief{{"E-CHK-008", "", c.cmd + " was stopped by signal 15 (terminated)"}}, briefs(c.problems(r)), c.cmd)
	}
}

func TestBuildProblems(t *testing.T) {
	for name, c := range map[string]struct {
		r    result
		want []brief
	}{
		"two packages, sorted by position": {ran("go build", `{"ImportPath":"app/pages","Action":"build-output","Output":"# app/pages\n"}
{"ImportPath":"app/pages","Action":"build-output","Output":"pages/x.go:6:2: undefined: undefinedPage\n"}
{"ImportPath":"app/pages","Action":"build-fail"}
{"ImportPath":"app","Action":"build-output","Output":"# app\n"}
{"ImportPath":"app","Action":"build-output","Output":"./main.go:4:2: undefined: undefinedMain\n"}
{"ImportPath":"app","Action":"build-output","Output":"./main.go:10:2: undefined: other\n"}
{"ImportPath":"app","Action":"build-fail"}
`, "", true), []brief{
			{"E-CHK-002", "main.go:4", "undefined: undefinedMain"},
			{"E-CHK-002", "main.go:10", "undefined: other"},
			{"E-CHK-002", "pages/x.go:6", "undefined: undefinedPage"},
		}},
		"continuation lines": {ran("go build", `{"ImportPath":"example.com/nothere","Action":"build-output","Output":"main.go:3:8: missing go.sum entry for module providing package example.com/nothere (imported by app); to add:\n"}
{"ImportPath":"example.com/nothere","Action":"build-output","Output":"\tgo get app\n"}
{"ImportPath":"example.com/nothere","Action":"build-fail"}
`, "", true), []brief{
			{"E-CHK-002", "main.go:3", "missing go.sum entry for module providing package example.com/nothere (imported by app); to add:\n\tgo get app"},
		}},
		"absolute paths": {ran("go build", `{"ImportPath":"app","Action":"build-output","Output":"/work/app/pages/x.go:6: undefined: x\n/elsewhere/y.go:3:1: bad\n"}
{"ImportPath":"app","Action":"build-fail"}
`, "", true), []brief{
			{"E-CHK-002", "/elsewhere/y.go:3", "bad"},
			{"E-CHK-002", "pages/x.go:6", "undefined: x"},
		}},
		"no position": {ran("go build", `{"ImportPath":"./...","Action":"build-output","Output":"pattern ./...: directory prefix . does not contain main module or its selected dependencies\n"}
{"ImportPath":"./...","Action":"build-fail"}
`, "", true), []brief{
			{"E-CHK-002", "", "go build of ./... failed: pattern ./...: directory prefix . does not contain main module or its selected dependencies"},
		}},
		"output of a package that built": {ran("go build", `{"ImportPath":"app","Action":"build-output","Output":"# app\nmain.go:3:1: warning: cgo says so\n"}
`, "", false), []brief{}},
		"only standard error": {ran("go build", "", "go: go.mod file not found in current directory or any parent directory; see 'go help modules'\n", true), []brief{
			{"E-CHK-002", "", "go build failed: go: go.mod file not found in current directory or any parent directory; see 'go help modules'"},
		}},
		"nothing printed": {ran("go build", "", "", true), []brief{{"E-CHK-002", "", "go build failed"}}},
	} {
		assert.Equal(t, c.want, briefs(buildProblems(c.r)), name)
	}
}

func TestVetProblems(t *testing.T) {
	const stderr = `# app/pages
vet: pages/p.go:10:12: undefined: undefinedY
# app
# [app]
{
	"app": {
		"printf": [
			{
				"posn": "/work/app/main.go:6:14",
				"message": "fmt.Printf format %d has arg \"text\" of wrong type string"
			}
		],
		"copylocks": {
			"error": "analysis failed"
		}
	}
}
# app/lib
{}
`
	assert.Equal(t, []brief{
		{"E-CHK-003", "", "copylocks: analysis failed"},
		{"E-CHK-003", "main.go:6", `printf: fmt.Printf format %d has arg "text" of wrong type string`},
		{"E-CHK-002", "pages/p.go:10", "undefined: undefinedY"},
	}, briefs(vetProblems(ran("go vet", "", stderr, true))))

	assert.Equal(t, []brief{
		{"E-CHK-003", "main.go:6", "printf: wrong"},
		{"E-CHK-002", "pages/p.go:10", "undefined: undefinedY"},
	}, briefs(vetProblems(ran("go vet", "{\n\t\"app\": {\"printf\": [{\"posn\": \"/work/app/main.go:6:14\", \"message\": \"wrong\"}]}\n}\n",
		"# app/pages\nvet: pages/p.go:10:12: undefined: undefinedY\n", true))), "findings on stdout, as Go 1.26 prints them")

	assert.Empty(t, vetProblems(ran("go vet", "", "# app\n# [app]\n{}\n", false)))
	assert.Empty(t, vetProblems(ran("go vet", "{}\n", "go: downloading example.com/x v1.0.0\n", false)), "a download notice")
	const unread = "go vet printed what aicoded check cannot read, so its findings are unknown: "
	for _, c := range []struct{ stdout, stderr, unread string }{
		{"main.go:6:14: a finding in a form of its own\n", "", "main.go:6:14: a finding in a form of its own"},
		{"{\n\t\"app\": {\"printf\": [\n", "", "{\n\t\"app\": {\"printf\": ["},
		{`{"app": ["x"]}` + "\n", "", `{"app": ["x"]}`},
		{"", "# app\n# [app]\n[]\n", "[]"},
	} {
		assert.Equal(t, []brief{{"E-CHK-002", "", unread + c.unread}}, briefs(vetProblems(ran("go vet", c.stdout, c.stderr, false))),
			"vet printed %q and %q and passed", c.stdout, c.stderr)
	}
	assert.Equal(t, []brief{{"E-CHK-002", "main.go:3", "package app/nope is not in std"}},
		briefs(vetProblems(ran("go vet", "", "main.go:3:8: package app/nope is not in std\n", true))), "a package that does not load")
}

// testRun is what go test -json printed for packages that pass, fail, panic, time out and do not
// build, trimmed to the fields the parser reads.
const testRun = `{"Action":"start","Package":"app/calc"}
{"Action":"run","Package":"app/calc","Test":"TestAdd"}
{"Action":"output","Package":"app/calc","Test":"TestAdd","Output":"=== RUN   TestAdd\n"}
{"Action":"output","Package":"app/calc","Test":"TestAdd","Output":"    calc_test.go:6: noise from a test that passes\n"}
{"Action":"output","Package":"app/calc","Test":"TestAdd","Output":"--- PASS: TestAdd (0.00s)\n"}
{"Action":"pass","Package":"app/calc","Test":"TestAdd"}
{"Action":"run","Package":"app/calc","Test":"TestAddWrong"}
{"Action":"output","Package":"app/calc","Test":"TestAddWrong","Output":"=== RUN   TestAddWrong\n"}
{"Action":"output","Package":"app/calc","Test":"TestAddWrong","Output":"    calc_test.go:14: Add(2, 2) = 4, want 5\n"}
{"Action":"output","Package":"app/calc","Test":"TestAddWrong","Output":"--- FAIL: TestAddWrong (0.00s)\n"}
{"Action":"fail","Package":"app/calc","Test":"TestAddWrong"}
{"Action":"run","Package":"app/calc","Test":"TestSub"}
{"Action":"output","Package":"app/calc","Test":"TestSub","Output":"=== RUN   TestSub\n"}
{"Action":"run","Package":"app/calc","Test":"TestSub/inner"}
{"Action":"output","Package":"app/calc","Test":"TestSub/inner","Output":"=== RUN   TestSub/inner\n"}
{"Action":"output","Package":"app/calc","Test":"TestSub/inner","Output":"    calc_test.go:20: inner fails\n"}
{"Action":"output","Package":"app/calc","Test":"TestSub/inner","Output":"    --- FAIL: TestSub/inner (0.00s)\n"}
{"Action":"fail","Package":"app/calc","Test":"TestSub/inner"}
{"Action":"output","Package":"app/calc","Test":"TestSub","Output":"--- FAIL: TestSub (0.00s)\n"}
{"Action":"fail","Package":"app/calc","Test":"TestSub"}
{"Action":"output","Package":"app/calc","Output":"FAIL\n"}
{"Action":"output","Package":"app/calc","Output":"FAIL\tapp/calc\t0.002s\n"}
{"Action":"fail","Package":"app/calc"}
{"Action":"run","Package":"app/p","Test":"TestPanics"}
{"Action":"output","Package":"app/p","Test":"TestPanics","Output":"=== RUN   TestPanics\n"}
{"Action":"output","Package":"app/p","Test":"TestPanics","Output":"--- FAIL: TestPanics (0.00s)\n"}
{"Action":"output","Package":"app/p","Test":"TestPanics","Output":"panic: assignment to entry in nil map [recovered, repanicked]\n"}
{"Action":"output","Package":"app/p","Test":"TestPanics","Output":"\n"}
{"Action":"output","Package":"app/p","Test":"TestPanics","Output":"goroutine 34 [running]:\n"}
{"Action":"output","Package":"app/p","Test":"TestPanics","Output":"app/p.TestPanics(0xc000196540?)\n"}
{"Action":"output","Package":"app/p","Test":"TestPanics","Output":"\t/work/app/p/p_test.go:9 +0x28\n"}
{"Action":"fail","Package":"app/p","Test":"TestPanics"}
{"Action":"output","Package":"app/p","Output":"FAIL\tapp/p\t0.005s\n"}
{"Action":"fail","Package":"app/p"}
{"Action":"run","Package":"app/slow","Test":"TestSlow"}
{"Action":"output","Package":"app/slow","Test":"TestSlow","Output":"=== RUN   TestSlow\n"}
{"Action":"output","Package":"app/slow","Test":"TestSlow","Output":"panic: test timed out after 2s\n"}
{"Action":"output","Package":"app/slow","Test":"TestSlow","Output":"\trunning tests:\n"}
{"Action":"output","Package":"app/slow","Test":"TestSlow","Output":"\t\tTestSlow (2s)\n"}
{"Action":"output","Package":"app/slow","Test":"TestSlow","Output":"app/slow.TestSlow(0xc000003a40?)\n"}
{"Action":"output","Package":"app/slow","Test":"TestSlow","Output":"\t/work/app/slow/slow_test.go:9 +0x1d\n"}
{"Action":"output","Package":"app/slow","Output":"FAIL\tapp/slow\t2.007s\n"}
{"Action":"fail","Package":"app/slow"}
{"Action":"output","Package":"app","Output":"panic: setup broke\n"}
{"Action":"output","Package":"app","Output":"app.TestMain(...)\n"}
{"Action":"output","Package":"app","Output":"\t/work/app/main_test.go:6\n"}
{"Action":"output","Package":"app","Output":"main.main()\n"}
{"Action":"output","Package":"app","Output":"\t_testmain.go:47 +0xaa\n"}
{"Action":"output","Package":"app","Output":"FAIL\tapp\t0.004s\n"}
{"Action":"fail","Package":"app"}
{"ImportPath":"app/b [app/b.test]","Action":"build-output","Output":"# app/b [app/b.test]\n"}
{"ImportPath":"app/b [app/b.test]","Action":"build-output","Output":"b/b_test.go:5:2: undefined: nope\n"}
{"ImportPath":"app/b [app/b.test]","Action":"build-fail"}
{"Action":"start","Package":"app/b"}
{"Action":"output","Package":"app/b","Output":"FAIL\tapp/b [build failed]\n"}
{"Action":"fail","Package":"app/b","FailedBuild":"app/b [app/b.test]"}
{"Action":"output","Package":"app/ok","Output":"ok  \tapp/ok\t(cached)\n"}
{"Action":"pass","Package":"app/ok"}
`

func TestTestProblems(t *testing.T) {
	assert.Equal(t, []brief{
		{"E-CHK-002", "b/b_test.go:5", "undefined: nope"},
		{"E-CHK-004", "calc/calc_test.go:14", "TestAddWrong failed: calc_test.go:14: Add(2, 2) = 4, want 5"},
		{"E-CHK-004", "calc/calc_test.go:20", "TestSub failed: calc_test.go:20: inner fails"},
		{"E-CHK-004", "main_test.go:6", "package app failed: panic: setup broke\napp.TestMain(...)\n\t/work/app/main_test.go:6\nmain.main()\n\t_testmain.go:47 +0xaa"},
		{"E-CHK-004", "p/p_test.go:9", "TestPanics failed: panic: assignment to entry in nil map [recovered, repanicked]\ngoroutine 34 [running]:\napp/p.TestPanics(0xc000196540?)\n\t/work/app/p/p_test.go:9 +0x28"},
		{"E-CHK-004", "slow/slow_test.go:9", "package app/slow failed: panic: test timed out after 2s\n\trunning tests:\n\t\tTestSlow (2s)\napp/slow.TestSlow(0xc000003a40?)\n\t/work/app/slow/slow_test.go:9 +0x1d"},
	}, briefs(testProblems(ran("go test", testRun, "", true), "app")))
}

func TestTestProblemsCutOutput(t *testing.T) {
	var out strings.Builder
	for range 30 {
		out.WriteString(`{"Action":"output","Package":"app","Test":"TestLoud","Output":"    x_test.go:3: line\n"}` + "\n")
	}
	out.WriteString(`{"Action":"fail","Package":"app","Test":"TestLoud"}` + "\n")
	ps := testProblems(ran("go test", out.String(), "", true), "app")
	assert.Len(t, ps, 1)
	assert.Equal(t, "TestLoud failed: "+strings.TrimSuffix(strings.Repeat("x_test.go:3: line\n", 20), "\n"), ps[0].Msg, "the first 20 lines")

	out.Reset()
	out.WriteString(`{"Action":"output","Package":"app","Test":"TestLong","Output":"    x_test.go:4: ` + strings.Repeat("é", 3000) + `\n"}` + "\n")
	out.WriteString(`{"Action":"fail","Package":"app","Test":"TestLong"}` + "\n")
	ps = testProblems(ran("go test", out.String(), "", true), "app")
	assert.LessOrEqual(t, len(ps[0].Msg), len("TestLong failed: ")+maxOutput+len(" …"))
	assert.True(t, strings.HasSuffix(ps[0].Msg, "é …"), "cut at a character boundary")
}

func TestTestProblemsWithoutEvents(t *testing.T) {
	assert.Equal(t, []brief{{"E-CHK-002", "", "go test failed: go: cannot find main module"}},
		briefs(testProblems(ran("go test", "", "go: cannot find main module\n", true), "app")))
	assert.Empty(t, testProblems(ran("go test", `{"Action":"pass","Package":"app"}`+"\n", "", false), "app"))
}

func TestParseModule(t *testing.T) {
	for data, want := range map[string]string{
		"module shop\n\ngo 1.25.0\n":                 "shop",
		"// a comment\nmodule \"ex.com/a b\" // x\n": "ex.com/a b",
		"go 1.25.0\n": "",
		"modules x\n": "",
	} {
		assert.Equal(t, want, parseModule([]byte(data)), data)
	}
}

const fw = "aicoded.dev/framework"

// failedBuild is a run of go build -json in which the package pkg failed with the lines of output.
func failedBuild(pkg string, lines ...string) result {
	var out strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&out, "{\"ImportPath\":%q,\"Action\":\"build-output\",\"Output\":%q}\n", pkg, l+"\n")
	}
	fmt.Fprintf(&out, "{\"ImportPath\":%q,\"Action\":\"build-fail\"}\n", pkg)
	return ran("go build", out.String(), "", true)
}

func TestFrameworkHint(t *testing.T) {
	for name, c := range map[string]struct {
		r             result
		pos, msg, fix string
	}{
		"no required module": {
			failedBuild(fw+"/form", "pages/new.go:5:2: no required module provides package "+fw+"/form; to add it:", "\tgo get "+fw+"/form"),
			"pages/new.go:5",
			"no required module provides package " + fw + "/form; to add it:\n\tgo get " + fw + "/form\n" +
				"the framework has no package " + fw + "/form; its form package is " + fw + "/web/form",
			`import "` + fw + `/web/form" instead`,
		},
		"latest does not contain it": {
			failedBuild(fw+"/reactive", "pages/new.go:6:2: module "+fw+"@latest found (v0.3.0), but does not contain package "+fw+"/reactive"),
			"pages/new.go:6",
			"module " + fw + "@latest found (v0.3.0), but does not contain package " + fw + "/reactive\n" +
				"the framework has no package " + fw + "/reactive; its reactive package is " + fw + "/web/reactive",
			`import "` + fw + `/web/reactive" instead`,
		},
		"cannot find": {
			failedBuild(fw+"/web/mailer", `main.go:4:2: cannot find package "`+fw+`/web/mailer" in any of:`, "\t/usr/local/go/src/"+fw+"/web/mailer (from $GOROOT)"),
			"main.go:4",
			`cannot find package "` + fw + `/web/mailer" in any of:` + "\n\t/usr/local/go/src/" + fw + "/web/mailer (from $GOROOT)\n" +
				"the framework has no package " + fw + "/web/mailer; its mailer package is " + fw + "/mailer",
			`import "` + fw + `/mailer" instead`,
		},
		"cannot find module": {
			failedBuild(fw+"/form", "main.go:3:8: cannot find module providing package "+fw+"/form: module lookup disabled by GOPROXY=off"),
			"main.go:3",
			"cannot find module providing package " + fw + "/form: module lookup disabled by GOPROXY=off\n" +
				"the framework has no package " + fw + "/form; its form package is " + fw + "/web/form",
			`import "` + fw + `/web/form" instead`,
		},
		"no package of that name": {
			failedBuild(fw+"/forms", "pages/new.go:5:2: no required module provides package "+fw+"/forms; to add it:", "\tgo get "+fw+"/forms"),
			"pages/new.go:5",
			"no required module provides package " + fw + "/forms; to add it:\n\tgo get " + fw + "/forms",
			fixBuild,
		},
		"beside the framework": {
			failedBuild(fw+"x/form", "pages/new.go:5:2: no required module provides package "+fw+"x/form; to add it:", "\tgo get "+fw+"x/form"),
			"pages/new.go:5",
			"no required module provides package " + fw + "x/form; to add it:\n\tgo get " + fw + "x/form",
			fixBuild,
		},
		"not the framework": {
			failedBuild("example.com/form", "pages/new.go:5:2: no required module provides package example.com/form; to add it:", "\tgo get example.com/form"),
			"pages/new.go:5",
			"no required module provides package example.com/form; to add it:\n\tgo get example.com/form",
			fixBuild,
		},
	} {
		ps := buildProblems(c.r)
		require.Len(t, ps, 1, name)
		assert.Equal(t, brief{codeBuild, c.pos, c.msg}, briefs(ps)[0], name)
		assert.Equal(t, c.fix, ps[0].Fix, name)
	}
}

func TestFrameworkHintNamesEveryMatch(t *testing.T) {
	p := errs.At("main.go:3", codeBuild, "no required module provides package "+fw+"/form; to add it:", fixBuild)
	frameworkHint(p, []string{fw + "/web/form", fw + "/admin/form", fw + "/web"})
	assert.Equal(t, "no required module provides package "+fw+"/form; to add it:\n"+
		"the framework has no package "+fw+"/form; its form packages are "+fw+"/web/form and "+fw+"/admin/form", p.Msg)
	assert.Equal(t, `import "`+fw+`/web/form" or "`+fw+`/admin/form" instead`, p.Fix)
}

// A package the framework has gets no hint, even when another package has its name: the app
// requires a framework without it.
func TestFrameworkHintSkipsItsOwnPackages(t *testing.T) {
	p := errs.At("main.go:3", codeBuild, "no required module provides package "+fw+"/web/form; to add it:", fixBuild)
	frameworkHint(p, []string{fw + "/web/form", fw + "/admin/form"})
	assert.Equal(t, "no required module provides package "+fw+"/web/form; to add it:", p.Msg)
	assert.Equal(t, fixBuild, p.Fix)
}

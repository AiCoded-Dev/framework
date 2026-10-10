// Command aicoded builds, runs and checks apps made with the aicoded framework.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/describe"
	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/explain"
	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/l7runner"
	"aicoded.dev/framework/cmd/aicoded/internal/rpcadd"
	"aicoded.dev/framework/cmd/aicoded/internal/scaffold"
	"aicoded.dev/framework/docs"
	"aicoded.dev/framework/internal/errs"
)

const usage = `usage: aicoded <command> [arguments]

commands:
  check [dir]     generate, build, vet, lint and test every app in dir or below it (--frozen, --no-tests, --json, --app <name>)
  describe [dir]  summarise what every app in dir or below it serves, calls and reaches (--json, --app <name>)
  dev [dir]       run every app in dir or below it (default: current directory) on a local runner (--manual <app>)
  explain [topic] print a page of the docs, such as guides/overview, changelog or E-DEV-001, or list them all
  generate [dir]  generate the code of the app in dir from pages/, rpc/ and .aicoded/services/
  init <name>     create the app <name> in a new folder of that name in the current directory
  login           sign in to the platform, in the browser or with a code (--org <organisation>, --device)
  logout          revoke the sign-in to the platform and forget it
  mcp [dir]       serve the AI assistant's tools for the apps in dir over MCP on stdin and stdout
  publish [dir]   send the commit of the app in dir to the platform's delivery pipeline and wait for the outcome (--app <name>, -m <summary>, --no-wait)
  rpc add <app>   let the app in the current directory call the functions of the app named <app>
  status <id>     print the steps, problems and outcome of a publish (--json)
  version         print the aicoded version
  whoami          print the organisation, email and scopes of the sign-in to the platform (--json)
`

// Exit codes: success, a failure the command reports, and a command line that is not valid.
const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

var version = "devel"

// frameworkDir is the framework checkout this aicoded was installed from by make install, or "".
var frameworkDir string

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "check":
		return runCheck(args, stdout, stderr)
	case "describe":
		return runDescribe(args, stdout, stderr)
	case "dev":
		fs := newFlags("dev", stderr)
		var manual names
		fs.Var(&manual, "manual", "run the `app` by hand, as under a debugger: aicoded dev keeps its runner up and prints the command; repeat it for more apps")
		rest, code, ok := parseArgs(fs, args, 0, 1)
		if !ok {
			return code
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := dev.Run(ctx, dirArg(rest), stdout, dev.Options{Manual: manual, Login: true}); err != nil {
			code := fail(stderr, err)
			if errs.Code(err) == "E-DEV-012" {
				if link, ok := dev.RunningLogin(ctx, dirArg(rest)); ok {
					fmt.Fprintf(stderr, "aicoded dev: log in to the running one at %s\n", link)
				}
			}
			return code
		}
		return exitOK
	case "explain":
		rest, code, ok := parseArgs(newFlags("explain", stderr), args, 0, 1)
		if !ok {
			return code
		}
		if len(rest) == 0 {
			topics, err := docs.Topics()
			if err != nil {
				return fail(stderr, err)
			}
			entries, err := explain.List()
			if err != nil {
				return fail(stderr, err)
			}
			for _, t := range topics {
				fmt.Fprintf(stdout, "%s  %s\n", t.Path, t.Title)
			}
			for _, e := range entries {
				fmt.Fprintf(stdout, "%s  %s\n", e.Code, e.Title)
			}
			return exitOK
		}
		page, err := explain.Read(rest[0])
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprint(stdout, page)
		return exitOK
	case "generate":
		rest, code, ok := parseArgs(newFlags("generate", stderr), args, 0, 1)
		if !ok {
			return code
		}
		res, err := generate.Run(dirArg(rest), generate.Options{})
		for _, w := range res.Warnings {
			fmt.Fprintln(stderr, "aicoded generate: warning: "+w)
		}
		if err != nil {
			return fail(stderr, err)
		}
		return exitOK
	case "init":
		rest, code, ok := parseArgs(newFlags("init", stderr), args, 1, 1)
		if !ok {
			return code
		}
		fw, err := scaffold.Detect(frameworkDir)
		if err != nil {
			return fail(stderr, err)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if _, err := scaffold.Create(ctx, ".", rest[0], fw, stdout); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	case "l7-runner":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return l7runner.Main(ctx, args, stderr)
	case "login":
		return runLogin(args, stdout, stderr)
	case "logout":
		return runLogout(args, stdout, stderr)
	case "mcp":
		return runMCP(args, stdout, stderr)
	case "publish":
		return runPublish(args, stdout, stderr)
	case "rpc":
		if len(args) == 0 || args[0] != "add" {
			fmt.Fprint(stderr, usage)
			return exitUsage
		}
		rest, code, ok := parseArgs(newFlags("rpc add", stderr), args[1:], 1, 1)
		if !ok {
			return code
		}
		if err := rpcadd.Add(".", rest[0], stdout); err != nil {
			return fail(stderr, err)
		}
		return exitOK
	case "status":
		return runStatus(args, stdout, stderr)
	case "version":
		if _, code, ok := parseArgs(newFlags("version", stderr), args, 0, 0); !ok {
			return code
		}
		fmt.Fprintln(stdout, "aicoded", version)
		return exitOK
	case "whoami":
		return runWhoami(args, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "aicoded: unknown command %q\n\n%s", cmd, usage)
		return exitUsage
	}
}

// newFlags returns the flag set of the command name, which reports its errors and its usage to
// stderr.
func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("aicoded "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		n := 0
		fs.VisitAll(func(*flag.Flag) { n++ })
		if n > 0 {
			fmt.Fprintf(stderr, "\nflags of %s:\n", fs.Name())
			fs.PrintDefaults()
		}
	}
	return fs
}

// parseArgs parses args with fs and returns the arguments after the flags, of which there must
// be between minArgs and maxArgs. When the command must not run, ok is false and code is the exit
// code: exitOK after -h, and exitUsage for a flag or a number of arguments that is not valid.
func parseArgs(fs *flag.FlagSet, args []string, minArgs, maxArgs int) (rest []string, code int, ok bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, exitOK, false
		}
		return nil, exitUsage, false
	}
	switch {
	case fs.NArg() < minArgs:
		fmt.Fprintf(fs.Output(), "%s: missing argument\n\n", fs.Name())
	case fs.NArg() > maxArgs:
		fmt.Fprintf(fs.Output(), "%s: too many arguments\n\n", fs.Name())
	default:
		return fs.Args(), exitOK, true
	}
	fs.Usage()
	return nil, exitUsage, false
}

// names is a flag that may be given more than once.
type names []string

func (n *names) String() string { return strings.Join(*n, ",") }

func (n *names) Set(v string) error {
	*n = append(*n, v)
	return nil
}

// dirArg returns the folder a command works on: its one argument, or the current folder.
func dirArg(rest []string) string {
	if len(rest) > 0 {
		return rest[0]
	}
	return "."
}

// fail prints err and returns exitFail.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, err)
	return exitFail
}

// runCheck runs aicoded check with the arguments after the command.
func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("check", stderr)
	frozen := fs.Bool("frozen", false, "write nothing, and fail on generated files that are out of date")
	noTests := fs.Bool("no-tests", false, "skip the tests: generate, build, vet and lint only")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	app := fs.String("app", "", "check only the app with this name")
	rest, code, ok := parseArgs(fs, args, 0, 1)
	if !ok {
		return code
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fw, _ := scaffold.Detect(frameworkDir)
	report, err := check.Run(ctx, dirArg(rest), check.Options{Frozen: *frozen, NoTests: *noTests, App: *app, FrameworkDir: fw.Dir})
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(report)
	} else {
		err = report.WriteText(stdout)
	}
	if err != nil {
		return fail(stderr, err)
	}
	if !report.OK() {
		return exitFail
	}
	return exitOK
}

// runDescribe runs aicoded describe with the arguments after the command.
func runDescribe(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("describe", stderr)
	asJSON := fs.Bool("json", false, "print the summary as JSON")
	app := fs.String("app", "", "describe only the app with this name")
	rest, code, ok := parseArgs(fs, args, 0, 1)
	if !ok {
		return code
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	summary, err := describe.Describe(ctx, dirArg(rest), *app)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(summary)
	} else {
		err = summary.WriteText(stdout)
	}
	if err != nil {
		return fail(stderr, err)
	}
	return exitOK
}

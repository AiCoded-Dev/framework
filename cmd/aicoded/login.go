package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
)

// openURL opens the sign-in in the person's browser.
var openURL = platform.OpenBrowser

// platformClient returns a client of the platform that AICODED_PLATFORM names, which tells the
// person what to do to sign in on stdout.
func platformClient(stdout io.Writer) (*platform.Client, error) {
	address, err := platform.Address()
	if err != nil {
		return nil, err
	}
	c := platform.New(address, stdout)
	c.Open = openURL
	return c, nil
}

// runLogin runs aicoded login with the arguments after the command.
func runLogin(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("login", stderr)
	org := fs.String("org", "", "the `organisation` to sign in to; the first login needs it, and later ones remember it")
	device := fs.Bool("device", false, "sign in with a code entered in a browser on any device, instead of a browser on this computer")
	if _, code, ok := parseArgs(fs, args, 0, 0); !ok {
		return code
	}
	if *org != "" && !platform.ValidOrg(*org) {
		fmt.Fprint(stderr, "aicoded login: --org must name an organisation: 2 to 39 lowercase letters, digits and dashes\n\n")
		fs.Usage()
		return exitUsage
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	who, err := c.Login(ctx, *org, *device)
	if errors.Is(err, platform.ErrNoOrg) {
		fmt.Fprint(stderr, "aicoded login: the first login needs --org <organisation>\n\n")
		fs.Usage()
		return exitUsage
	}
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "\nSigned in to %s as %s.\n", who.Org, who.Email)
	return exitOK
}

// runLogout runs aicoded logout with the arguments after the command.
func runLogout(args []string, stdout, stderr io.Writer) int {
	if _, code, ok := parseArgs(newFlags("logout", stderr), args, 0, 0); !ok {
		return code
	}
	address, err := platform.Address()
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	signedIn, revoked, err := platform.New(address, stdout).Logout(ctx)
	switch {
	case err != nil:
		return fail(stderr, err)
	case !signedIn:
		fmt.Fprintf(stdout, "Not signed in to %s.\n", address)
	case revoked:
		fmt.Fprintf(stdout, "Signed out of %s.\n", address)
	default:
		fmt.Fprintf(stdout, "Signed out of %s on this computer, but the platform did not revoke the sign-in: "+
			"it stays valid there until it expires, within 30 days.\n", address)
	}
	return exitOK
}

// runWhoami runs aicoded whoami with the arguments after the command.
func runWhoami(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("whoami", stderr)
	asJSON := fs.Bool("json", false, "print the sign-in as JSON")
	if _, code, ok := parseArgs(fs, args, 0, 0); !ok {
		return code
	}
	c, err := platformClient(stdout)
	if err != nil {
		return fail(stderr, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	who, err := c.Whoami(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(who)
	} else {
		_, err = fmt.Fprintf(stdout, "org: %s\nemail: %s\nscopes: %s\n", who.Org, who.Email, strings.Join(who.Scopes, " "))
	}
	if err != nil {
		return fail(stderr, err)
	}
	return exitOK
}

// Command eyeball is the review surface and the command line that drives it.
//
// One binary. Each subcommand is the client an agent uses, talking to the
// daemon over a unix socket, except serve, which is the daemon. With no
// subcommand it prints help: an agent running eyeball to see what it does would
// otherwise block in a foreground daemon.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/noonat/eyeball/internal/cli"
	"github.com/noonat/eyeball/internal/daemon"
)

func main() {
	if err := run(os.Args); err != nil {
		// The command line prints its own refusals and returns a status. Any
		// other error came from here, before it ran, and nothing has said it.
		var status *cli.Status
		if !errors.As(err, &status) {
			fmt.Fprintf(os.Stderr, "eyeball: %v\n", err)
		}
		os.Exit(cli.ExitCode(err))
	}
}

// run is everything main does, so that a failure returns rather than exiting.
//
// os.Exit from wherever a call happened to fail skips every deferred close, and
// scatters the decision about what a failure looks like across a dozen sites.
// The command line prints its own refusals and returns only a status. An error
// from finding the working directory or the binary comes back unprinted.
func run(args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	// The shell's spelling of the directory, through any symlinks, is what an
	// agent builds absolute paths from.
	if pwd := os.Getenv("PWD"); pwd != "" && sameDir(pwd, dir) {
		dir = pwd
	}
	home, _ := os.UserHomeDir()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return cli.Run(context.Background(), args, cli.Env{
		Dir:    dir,
		Getenv: os.Getenv,
		Home:   home,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Exe:    exe,
		Build:  daemon.Build(),
	})
}

// sameDir reports whether two paths name the same directory, so a stale PWD
// left by a parent process is not trusted over the real one.
func sameDir(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ia, ib)
}

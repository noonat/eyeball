// Command eyeball is the review surface and the command line that drives it.
//
// One binary. With a subcommand it acts as the client an agent uses, talking to
// the daemon over a unix socket. With none, or with serve, it is the daemon.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "eyeball: %+v\n", err)
		os.Exit(1)
	}
}

// run is everything main does, so that a failure returns rather than exiting.
//
// os.Exit from wherever a call happened to fail skips every deferred close, and
// scatters the decision about what a failure looks like across a dozen sites.
func run(args []string) error {
	_ = args
	return nil
}

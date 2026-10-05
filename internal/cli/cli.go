package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/daemon"
	ucli "github.com/urfave/cli/v3"
)

// Env is everything the command line reads from the process it runs in, so a
// test can run it in-process with a directory, an environment and two buffers.
type Env struct {
	// Dir is the working directory, as the shell spells it.
	Dir string
	// Getenv reads an environment variable.
	Getenv func(string) string
	// Home is the user's home directory.
	Home string
	// Stdout and Stderr are where output goes.
	Stdout io.Writer
	// Stderr receives refusals in human mode, and warnings.
	Stderr io.Writer
	// Exe is the path of this binary, which is what a started daemon runs.
	Exe string
	// Build identifies this binary, compared with the daemon's.
	Build string
}

// Status is the exit status a command finished with, when it is not zero.
//
// It is the CLI's own type rather than urfave's ExitCoder because urfave
// builds an ExitCoder with status 3 for an unknown subcommand, which would
// collide with wait's 3. main honors this type and maps every other error to 1.
type Status struct {
	// Code is the exit status.
	Code int
}

// Error names the status.
func (s *Status) Error() string {
	return fmt.Sprintf("exit status %d", s.Code)
}

// ExitCode is the status main exits with for an error Run returned.
func ExitCode(err error) int {
	var s *Status
	if errors.As(err, &s) {
		return s.Code
	}
	return 1
}

// Exit statuses other than 0 and 1.
const (
	// exitStillWaiting is wait giving up at its --timeout.
	exitStillWaiting = 2
	// exitLateComments is wait returning comments while the round still
	// waits for a verdict.
	exitLateComments = 3
)

// usageError is a command line the CLI refuses before asking the daemon
// anything.
type usageError struct {
	message string
	fix     string
}

// Error is the refusal's message.
func (e *usageError) Error() string {
	return e.message
}

// failure is a command that ran and could not finish, with what to do about
// it. The daemon never sees these: the connection is what failed.
type failure struct {
	code    api.Code
	message string
	fix     string
}

// Error is the failure's message.
func (e *failure) Error() string {
	return e.message
}

// refused reports a refusal that has already been printed.
type refused struct{}

// Error says so.
func (refused) Error() string {
	return "refused"
}

// app is one run of the command line.
type app struct {
	env    Env
	out    output
	client *client
	agent  string
	// ran says a command's own action started, which separates an error from
	// urfave, raised while parsing, from one raised by the action.
	ran bool
}

// Run runs the command line and prints everything it has to say. It returns
// nil, or a *Status naming the exit status, so that main is the only place
// that exits and every deferred close runs first.
func Run(ctx context.Context, args []string, env Env) error {
	a := &app{env: env, out: output{stdout: env.Stdout, stderr: env.Stderr}}
	// A refusal raised while urfave parses comes before the root's Before
	// runs, so --json is found here too, or that refusal would print as text
	// to an agent parsing stdout.
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || arg == "-json" {
			a.out.json = true
		}
	}
	root := a.command()
	err := root.Run(ctx, args)
	if err == nil {
		return nil
	}
	var s *Status
	if errors.As(err, &s) {
		return s
	}
	if errors.As(err, &refused{}) {
		return &Status{Code: 1}
	}
	var (
		usage *usageError
		fail  *failure
	)
	switch {
	case errors.As(err, &usage):
		a.out.printRefusal(api.Error{Code: api.CodeUsage, Message: usage.message, Fix: usage.fix})
	case errors.As(err, &fail):
		a.out.printRefusal(api.Error{Code: fail.code, Message: fail.message, Fix: fail.fix})
	case errors.Is(err, errDropped):
		a.out.printRefusal(api.Error{Code: api.CodeConnectionLost, Message: err.Error(), Fix: "run the same command again; it starts a daemon if none is running"})
	case !a.ran:
		// urfave raised it before any action ran: an unknown subcommand, or
		// a command line it could not parse.
		message := err.Error()
		if topic, ok := strings.CutPrefix(message, "No help topic for "); ok {
			message = "there is no command " + topic
		}
		a.out.printRefusal(api.Error{Code: api.CodeUsage, Message: message, Fix: "run eyeball --help to list the commands, or eyeball doc for the manual"})
	default:
		a.out.printRefusal(api.Error{Code: api.CodeInternal, Message: err.Error(), Fix: "read the daemon's log in the state directory, or run eyeball doc for the manual"})
	}
	return &Status{Code: 1}
}

// command builds the command tree.
func (a *app) command() *ucli.Command {
	onUsage := func(_ context.Context, _ *ucli.Command, err error, _ bool) error {
		return &usageError{message: err.Error(), fix: "run eyeball <command> --help for its flags, or eyeball doc for the manual"}
	}
	commands := []*ucli.Command{
		a.requestCommand(),
		a.waitCommand(),
		a.showCommand(),
		a.statusCommand(),
		a.replyCommand(),
		a.abandonCommand(),
		a.startCommand(),
		a.stopCommand(),
		a.serveCommand(),
		a.docCommand(),
	}
	// v3 applies these per command, so the root's settings do not reach a
	// subcommand: a bad flag on request printed help to stdout until each
	// subcommand carried its own.
	for _, c := range commands {
		c.OnUsageError = onUsage
		c.DisableSliceFlagSeparator = true
		action := c.Action
		c.Action = func(ctx context.Context, cmd *ucli.Command) error {
			a.ran = true
			return action(ctx, cmd)
		}
	}
	return &ucli.Command{
		Name:                      "eyeball",
		Usage:                     "ask a person to review work, and wait for the verdict",
		Description:               "Run eyeball doc for the manual.",
		Writer:                    a.env.Stdout,
		ErrWriter:                 a.env.Stderr,
		ExitErrHandler:            func(context.Context, *ucli.Command, error) {},
		OnUsageError:              onUsage,
		DisableSliceFlagSeparator: true,
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "json", Usage: "print the answer as JSON"},
			&ucli.StringFlag{Name: "format", Usage: "print the answer through a Go template over its JSON"},
			&ucli.StringFlag{Name: "agent", Usage: "the name this agent goes by (default: EYEBALL_AGENT, then the project root's path)"},
		},
		Commands: commands,
		Before: func(ctx context.Context, cmd *ucli.Command) (context.Context, error) {
			a.out.json = cmd.Bool("json")
			a.out.format = cmd.String("format")
			if a.out.json && a.out.format != "" {
				return ctx, &usageError{message: "--json and --format print the same answer two ways", fix: "pass one of them"}
			}
			// Parsed here, before anything is sent, so a request is not made
			// and then reported as failed because its output template was bad.
			if a.out.format != "" {
				if _, err := template.New("format").Parse(a.out.format); err != nil {
					return ctx, &usageError{message: "the --format template does not parse: " + err.Error(), fix: "check the template's syntax; the field names are the ones --json prints"}
				}
			}
			a.agent = cmd.String("agent")
			if a.agent == "" {
				a.agent = a.env.Getenv("EYEBALL_AGENT")
			}
			return ctx, nil
		},
	}
}

// paths locates the state directory.
func (a *app) paths() (daemon.Paths, error) {
	return daemon.Locate(a.env.Getenv, a.env.Home)
}

// connect makes sure a daemon answers, starting one if start allows it, and
// warns when it runs a different binary from this one.
func (a *app) connect(ctx context.Context, start bool) error {
	paths, err := a.paths()
	if err != nil {
		return err
	}
	a.client = newClient(a.env, paths)
	if err := a.client.connect(ctx, start); err != nil {
		if errors.Is(err, errNoDaemon) && !start {
			return err
		}
		return &failure{code: api.CodeDaemonUnavailable, message: err.Error(), fix: "read the daemon's log in " + paths.Dir + ", or run eyeball serve to watch it start in the foreground"}
	}
	status, raw, err := a.client.call(ctx, http.MethodGet, "/agent/daemon", nil)
	if err != nil || status != http.StatusOK {
		return err
	}
	var info api.Daemon
	if json.Unmarshal(raw, &info) == nil && info.Build != a.env.Build {
		_, _ = fmt.Fprintf(a.env.Stderr, "eyeball: the running daemon is a different build (%s) from this command (%s); run eyeball stop, and the next command starts a new one\n", info.Build, a.env.Build)
	}
	return nil
}

// caller is who this command says it is.
func (a *app) caller() api.Caller {
	return api.Caller{Dir: a.env.Dir, Agent: a.agent}
}

// callerQuery is the caller as query parameters.
func (a *app) callerQuery() string {
	v := url.Values{}
	v.Set("dir", a.env.Dir)
	if a.agent != "" {
		v.Set("agent", a.agent)
	}
	return v.Encode()
}

// do sends a request to a daemon it starts if needed, and prints a refusal.
// It returns the body of a successful answer.
func (a *app) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	if err := a.connect(ctx, true); err != nil {
		return nil, err
	}
	status, raw, err := a.client.call(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		a.out.refuse(raw)
		return nil, refused{}
	}
	return raw, nil
}

// note joins every -m value with a blank line between them, as git commit
// does, and refuses a note that is blank throughout.
func note(cmd *ucli.Command, what string) (string, error) {
	parts := cmd.StringSlice("message")
	joined := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if joined == "" {
		return "", &usageError{message: what + " needs a note", fix: `pass -m "..."; repeat -m for more paragraphs`}
	}
	return joined, nil
}

// id reads the one positional argument a command takes as an id.
func id(cmd *ucli.Command, what string) (int64, error) {
	if cmd.Args().Len() != 1 {
		return 0, &usageError{message: fmt.Sprintf("eyeball %s takes one %s id", cmd.Name, what), fix: fmt.Sprintf("run eyeball %s <%s>", cmd.Name, what)}
	}
	n, err := strconv.ParseInt(cmd.Args().First(), 10, 64)
	if err != nil || n <= 0 {
		return 0, &usageError{message: fmt.Sprintf("%q is not a %s id", cmd.Args().First(), what), fix: "pass the number eyeball status lists"}
	}
	return n, nil
}

// messageFlag is the repeatable -m flag.
func messageFlag(usage string) ucli.Flag {
	return &ucli.StringSliceFlag{Name: "message", Aliases: []string{"m"}, Usage: usage}
}

// requestCommand opens a review or asks for its next round.
func (a *app) requestCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "request",
		Usage:     "open a review of paths, or ask for the next round of one",
		ArgsUsage: "<paths...>",
		Flags: []ucli.Flag{
			messageFlag("what to look at, or on a later round what changed (repeatable)"),
			&ucli.StringFlag{Name: "title", Usage: "the title the queue shows (default: the note's first line)"},
			&ucli.StringFlag{Name: "base", Usage: "the commit a first round is measured from (default: HEAD)"},
			&ucli.Int64Flag{Name: "review", Usage: "ask for the next round of this review instead of opening one"},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			text, err := note(cmd, "a request")
			if err != nil {
				return err
			}
			body := api.RequestBody{
				Caller: a.caller(),
				Paths:  cmd.Args().Slice(),
				Note:   text,
				Title:  cmd.String("title"),
				Base:   cmd.String("base"),
				Review: cmd.Int64("review"),
			}
			raw, err := a.do(ctx, http.MethodPost, "/agent/requests", body)
			if err != nil {
				return err
			}
			return a.out.emit(raw, func(w io.Writer) {
				var r api.RequestResult
				_ = json.Unmarshal(raw, &r)
				if r.Review.Round == 1 {
					_, _ = fmt.Fprintf(w, "review %d opened: %s\n", r.Review.ID, r.Review.Title)
				} else {
					_, _ = fmt.Fprintf(w, "review %d, round %d requested: %s\n", r.Review.ID, r.Review.Round, r.Review.Title)
				}
				_, _ = fmt.Fprintf(w, "project %s, agent %s\n", r.Review.Project.Name, r.Review.Agent)
				_, _ = fmt.Fprintf(w, "size: %s\n", sizeText(r.Size))
				printNext(w, r.Next)
			})
		},
	}
}

// waitCommand blocks until a review has something for the agent.
func (a *app) waitCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "wait",
		Usage:     "block until the verdict, then print it and every comment",
		ArgsUsage: "<review>",
		Flags: []ucli.Flag{
			&ucli.DurationFlag{Name: "timeout", Usage: "give up after this long and exit 2 (default: wait as long as it takes)"},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			reviewID, err := id(cmd, "review")
			if err != nil {
				return err
			}
			if err := a.connect(ctx, true); err != nil {
				return err
			}
			waitCtx := ctx
			if timeout := cmd.Duration("timeout"); timeout > 0 {
				var cancel context.CancelFunc
				waitCtx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			path := fmt.Sprintf("/agent/reviews/%d/wait?%s", reviewID, a.callerQuery())
			status, raw, err := a.client.call(waitCtx, http.MethodGet, path, nil)
			switch {
			case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
				return a.stillWaiting(reviewID)
			case errors.Is(err, errDropped):
				return &failure{code: api.CodeConnectionLost, message: err.Error(), fix: fmt.Sprintf("run eyeball wait %d again", reviewID)}
			case err != nil:
				return err
			case status != http.StatusOK:
				a.out.refuse(raw)
				return refused{}
			}
			var result api.WaitResult
			if err := json.Unmarshal(raw, &result); err != nil {
				return errors.Wrap(err, "read the answer")
			}
			if err := a.out.emit(raw, func(w io.Writer) { printWait(w, result) }); err != nil {
				return err
			}
			// The receipt goes after printing, so a command killed between
			// the two leaves the comments owed rather than lost. A template
			// may have printed none of them, so --format posts none.
			if a.out.format == "" && len(result.Comments) > 0 {
				ids := make([]int64, 0, len(result.Comments))
				for _, c := range result.Comments {
					ids = append(ids, c.ID)
				}
				receipt := api.ReceiptBody{Caller: a.caller(), Comments: ids}
				// The comments are printed already, so a receipt that fails
				// changes nothing the agent read. It leaves them owed, and the
				// next wait shows them again.
				status, _, err := a.client.call(ctx, http.MethodPost, fmt.Sprintf("/agent/reviews/%d/receipt", reviewID), receipt)
				if err != nil || status != http.StatusOK {
					_, _ = fmt.Fprintln(a.env.Stderr, "eyeball: the comments above were printed and not acknowledged, so the next wait shows them again")
				}
			}
			if result.Kind == api.KindComments {
				return &Status{Code: exitLateComments}
			}
			return nil
		},
	}
}

// stillWaiting prints the body a wait that gave up at its timeout reports.
// The client writes it: the daemon never saw the timeout.
func (a *app) stillWaiting(reviewID int64) error {
	result := api.WaitResult{
		Kind:     api.KindStillWaiting,
		Review:   api.Review{ID: reviewID},
		Comments: []api.Comment{},
		Next:     fmt.Sprintf("no verdict yet. Run eyeball wait %d again, and do not continue this work until it returns.", reviewID),
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return errors.Wrap(err, "encode the answer")
	}
	err = a.out.emit(raw, func(w io.Writer) {
		_, _ = fmt.Fprintf(w, "review %d: still waiting for a verdict\n", reviewID)
		printNext(w, result.Next)
	})
	if err != nil {
		return err
	}
	return &Status{Code: exitStillWaiting}
}

// printWait writes a wait's answer.
func printWait(w io.Writer, r api.WaitResult) {
	switch r.Kind {
	case api.KindVerdict:
		verdict := "approved"
		if r.Verdict == api.VerdictChanges {
			verdict = "changes requested"
		}
		_, _ = fmt.Fprintf(w, "review %d, round %d: %s\n", r.Review.ID, r.Review.Round, verdict)
	case api.KindAbandoned:
		_, _ = fmt.Fprintf(w, "review %d: withdrawn\n", r.Review.ID)
	case api.KindComments:
		_, _ = fmt.Fprintf(w, "review %d: %s arrived after an earlier verdict; round %d is still waiting\n", r.Review.ID, plural(len(r.Comments), "comment"), r.Review.Round)
	}
	if r.Note != "" {
		_, _ = fmt.Fprintf(w, "note: %s\n", r.Note)
	}
	for _, c := range r.Comments {
		printComment(w, c)
	}
	if len(r.Comments) > 0 {
		_, _ = fmt.Fprintln(w)
	}
	printNext(w, r.Next)
}

// showCommand reads any review's record.
func (a *app) showCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "show",
		Usage:     "print any review's rounds, verdicts and captured files, changing nothing",
		ArgsUsage: "<review>",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			reviewID, err := id(cmd, "review")
			if err != nil {
				return err
			}
			raw, err := a.do(ctx, http.MethodGet, fmt.Sprintf("/agent/reviews/%d", reviewID), nil)
			if err != nil {
				return err
			}
			return a.out.emit(raw, func(w io.Writer) {
				var r api.ShowResult
				_ = json.Unmarshal(raw, &r)
				_, _ = fmt.Fprintf(w, "review %d: %s\n", r.Review.ID, r.Review.Title)
				_, _ = fmt.Fprintf(w, "state %s, project %s, agent %s\n", r.Review.State, r.Review.Project.Name, r.Review.Agent)
				for _, round := range r.Rounds {
					decided := "waiting"
					if round.DecidedAt != nil {
						decided = fmt.Sprintf("%s at %s", round.Verdict, round.DecidedAt.Format(time.RFC3339))
					}
					_, _ = fmt.Fprintf(w, "round %d: %s, %s\n", round.Number, sizeText(round.Size), decided)
				}
				_, _ = fmt.Fprintf(w, "latest round captured %s:\n", plural(len(r.Files), "file"))
				for _, f := range r.Files {
					digest := "deleted"
					if f.Digest != "" {
						digest = f.Digest[:12]
					}
					_, _ = fmt.Fprintf(w, "  %s  %s\n", digest, f.Path)
				}
			})
		},
	}
}

// statusCommand lists this agent's reviews.
func (a *app) statusCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "status",
		Usage: "print the daemon and this agent's reviews in this project",
		Flags: []ucli.Flag{
			&ucli.BoolFlag{Name: "all", Usage: "list every review in the project, whichever agent opened it"},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			path := "/agent/status?" + a.callerQuery()
			if cmd.Bool("all") {
				path += "&all=1"
			}
			raw, err := a.do(ctx, http.MethodGet, path, nil)
			if err != nil {
				return err
			}
			return a.out.emit(raw, func(w io.Writer) {
				var r api.StatusResult
				_ = json.Unmarshal(raw, &r)
				started := ""
				if a.client.started {
					started = ", started by this command"
				}
				_, _ = fmt.Fprintf(w, "daemon pid %d, running since %s%s\n", r.Daemon.PID, r.Daemon.Started.Local().Format(time.RFC3339), started)
				_, _ = fmt.Fprintf(w, "project %s at %s, agent %s\n", r.Project.Name, r.Project.Root, r.Agent)
				if len(r.Reviews) == 0 {
					_, _ = fmt.Fprintln(w, "no reviews to show")
				}
				for _, row := range r.Reviews {
					unread := ""
					if row.Undelivered > 0 {
						unread = ", " + plural(row.Undelivered, "unread comment")
					}
					_, _ = fmt.Fprintf(w, "review %d, round %d, %s%s: %s (%s)\n", row.ID, row.Round, row.State, unread, row.Title, row.Agent)
				}
				printNext(w, r.Next)
			})
		},
	}
}

// replyCommand replies to one released comment.
func (a *app) replyCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "reply",
		Usage:     "reply to one comment, saying what was done about it",
		ArgsUsage: "<comment>",
		Flags:     []ucli.Flag{messageFlag("the reply (repeatable)")},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			commentID, err := id(cmd, "comment")
			if err != nil {
				return err
			}
			text, err := note(cmd, "a reply")
			if err != nil {
				return err
			}
			body := api.ReplyBody{Caller: a.caller(), Body: text}
			raw, err := a.do(ctx, http.MethodPost, fmt.Sprintf("/agent/comments/%d/reply", commentID), body)
			if err != nil {
				return err
			}
			return a.out.emit(raw, func(w io.Writer) {
				var r api.ReplyResult
				_ = json.Unmarshal(raw, &r)
				_, _ = fmt.Fprintf(w, "replied to comment %d\n", r.Comment.ID)
				printNext(w, r.Next)
			})
		},
	}
}

// abandonCommand withdraws a review.
func (a *app) abandonCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "abandon",
		Usage:     "withdraw a review whose question no longer needs an answer",
		ArgsUsage: "<review>",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			reviewID, err := id(cmd, "review")
			if err != nil {
				return err
			}
			body := api.AbandonBody{Caller: a.caller()}
			raw, err := a.do(ctx, http.MethodPost, fmt.Sprintf("/agent/reviews/%d/abandon", reviewID), body)
			if err != nil {
				return err
			}
			return a.out.emit(raw, func(w io.Writer) {
				var r api.AbandonResult
				_ = json.Unmarshal(raw, &r)
				_, _ = fmt.Fprintf(w, "review %d withdrawn: %s\n", r.Review.ID, r.Review.Title)
				printNext(w, r.Next)
			})
		},
	}
}

// startCommand starts a daemon if none is running.
func (a *app) startCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "start",
		Usage: "start the daemon if it is not running, and print it",
		Action: func(ctx context.Context, _ *ucli.Command) error {
			raw, err := a.do(ctx, http.MethodGet, "/agent/daemon", nil)
			if err != nil {
				return err
			}
			return a.out.emit(raw, func(w io.Writer) {
				var d api.Daemon
				_ = json.Unmarshal(raw, &d)
				verb := "already running"
				if a.client.started {
					verb = "started"
				}
				_, _ = fmt.Fprintf(w, "daemon %s: pid %d since %s, state in %s\n", verb, d.PID, d.Started.Local().Format(time.RFC3339), d.StateDir)
			})
		},
	}
}

// stopCommand asks the daemon to exit, and never starts one to do it.
func (a *app) stopCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "stop",
		Usage: "stop the daemon, and wait until it has gone",
		Action: func(ctx context.Context, _ *ucli.Command) error {
			err := a.connect(ctx, false)
			if errors.Is(err, errNoDaemon) {
				raw, merr := json.MarshalIndent(api.StopResult{Stopped: false}, "", "  ")
				if merr != nil {
					return errors.Wrap(merr, "encode the answer")
				}
				return a.out.emit(raw, func(w io.Writer) {
					_, _ = fmt.Fprintln(w, "no daemon is running")
				})
			}
			if err != nil {
				return err
			}
			status, raw, err := a.client.call(ctx, http.MethodPost, "/agent/stop", struct{}{})
			if err != nil {
				return err
			}
			if status != http.StatusOK {
				a.out.refuse(raw)
				return refused{}
			}
			// Waiting until nothing answers is what lets the next command
			// start a fresh daemon rather than reach the dying one.
			deadline := time.Now().Add(10 * time.Second)
			for a.client.dial(ctx) == nil {
				if time.Now().After(deadline) {
					return errors.New("the daemon was asked to stop and is still answering after 10s")
				}
				time.Sleep(20 * time.Millisecond)
			}
			return a.out.emit(raw, func(w io.Writer) {
				var r api.StopResult
				_ = json.Unmarshal(raw, &r)
				if r.Daemon != nil {
					_, _ = fmt.Fprintf(w, "daemon pid %d stopped\n", r.Daemon.PID)
				}
			})
		},
	}
}

// serveCommand runs a daemon in the foreground.
func (a *app) serveCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "serve",
		Usage: "run the daemon in the foreground (what start runs, and what a person debugging runs)",
		Action: func(ctx context.Context, _ *ucli.Command) error {
			paths, err := a.paths()
			if err != nil {
				return err
			}
			return daemon.Run(ctx, daemon.Config{
				Paths:  paths,
				Build:  a.env.Build,
				Home:   a.env.Home,
				Stderr: a.env.Stderr,
			})
		},
	}
}

// docCommand prints the manual. It arrives with the manual itself.
func (a *app) docCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "doc",
		Usage: "print the manual, which is enough to drive eyeball from",
		Action: func(_ context.Context, _ *ucli.Command) error {
			_, err := io.WriteString(a.env.Stdout, manual)
			return errors.WithStack(err)
		},
	}
}

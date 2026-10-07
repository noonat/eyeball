package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/daemon"
	"github.com/noonat/eyeball/internal/tailnet"
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
	// Terminal says stdin is a terminal. An agent's command runs without one,
	// though a process that allocates a pseudo-terminal can pass, so this
	// keeps an agent from starting a login by accident rather than by intent.
	Terminal bool
	// NewNode builds a tailnet node, and nil means tsnet. The tests' shared
	// environment sets one that fails if it is called, since a real node
	// contacts Tailscale.
	NewNode tailnet.Constructor
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
	case errors.Is(err, errOldDaemon):
		a.out.printRefusal(api.Error{Code: api.CodeDaemonUnavailable, Message: err.Error(), Fix: "the daemon is an older build than this command: run eyeball stop, and the next command starts the current one"})
	case errors.Is(err, errDropped):
		// The daemon may have acted before the connection dropped, so the fix
		// is to look before repeating: a request run twice opens two reviews.
		a.out.printRefusal(api.Error{Code: api.CodeConnectionLost, Message: err.Error(), Fix: "run eyeball status to see whether it took effect, and run the command again if it did not"})
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
		a.tailnetCommand(),
		a.docCommand(),
	}
	// v3 applies these per command, so the root's settings do not reach a
	// subcommand: a bad flag on request printed help to stdout until each
	// subcommand carried its own.
	var configure func(cs []*ucli.Command)
	configure = func(cs []*ucli.Command) {
		for _, c := range cs {
			c.OnUsageError = onUsage
			c.DisableSliceFlagSeparator = true
			if action := c.Action; action != nil {
				c.Action = func(ctx context.Context, cmd *ucli.Command) error {
					a.ran = true
					return action(ctx, cmd)
				}
			}
			configure(c.Commands)
		}
	}
	configure(commands)
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
		return &failure{code: api.CodeDaemonUnavailable, message: err.Error(), fix: unavailableFix(err, paths)}
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

// unavailableFix is what to do about a daemon that could not be reached or
// started, which depends on why.
func unavailableFix(err error, paths daemon.Paths) string {
	var (
		stuck *stuckError
		login *loginError
	)
	switch {
	case errors.As(err, &login):
		return "the owner is logging this machine in to the tailnet; run the command again once the login finishes"
	case errors.As(err, &stuck):
		return fmt.Sprintf("run kill %d, and the next command starts a new daemon", stuck.holder.PID)
	case errors.Is(err, daemon.ErrSocketPath):
		return "set XDG_STATE_HOME to a shorter directory"
	case errors.Is(err, daemon.ErrStateDirMode):
		return fmt.Sprintf("run chmod 700 %s if it should be private, or set XDG_STATE_HOME to another directory", paths.Dir)
	case errors.Is(err, errNoDaemon):
		return "run eyeball start, or unset EYEBALL_NO_START so commands start a daemon themselves"
	default:
		return "read eyeball.log in " + paths.Dir
	}
}

// caller is who this command says it is.
func (a *app) caller() api.Caller {
	return api.Caller{Dir: a.env.Dir, Agent: a.agent, Home: a.env.Home}
}

// callerQuery is the caller as query parameters.
func (a *app) callerQuery() string {
	v := url.Values{}
	v.Set("dir", a.env.Dir)
	v.Set("home", a.env.Home)
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

// viewFlags reads each --view as a path and a choice. The choice is split off
// at the last equals sign, since a path may hold one and a choice never does.
// Whether the choice names a view the file has is the daemon's to say, since
// only the capture knows the file.
func viewFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		i := strings.LastIndex(f, "=")
		if i <= 0 || i == len(f)-1 {
			return nil, &usageError{message: fmt.Sprintf("--view %q names no path and view", f), fix: "pass --view <path>=rendered or --view <path>=source"}
		}
		out[f[:i]] = f[i+1:]
	}
	return out, nil
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
			&ucli.StringSliceFlag{Name: "view", Usage: "open a file in its rendered or source view first, as <path>=rendered or <path>=source (repeatable)"},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			text, err := note(cmd, "a request")
			if err != nil {
				return err
			}
			views, err := viewFlags(cmd.StringSlice("view"))
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
				Views:  views,
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
				return a.stillWaiting(ctx, reviewID)
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
			if a.out.format == "" && (len(result.Comments) > 0 || result.Kind == api.KindVerdict) {
				ids := make([]int64, 0, len(result.Comments))
				for _, c := range result.Comments {
					ids = append(ids, c.ID)
				}
				receipt := api.ReceiptBody{Caller: a.caller(), Comments: ids}
				if result.Kind == api.KindVerdict {
					receipt.Round = result.Review.Round
				}
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
func (a *app) stillWaiting(ctx context.Context, reviewID int64) error {
	result := api.WaitResult{
		Kind:     api.KindStillWaiting,
		Review:   api.Review{ID: reviewID, Paths: []string{}},
		Comments: []api.Comment{},
		Next:     fmt.Sprintf("no verdict yet. Run eyeball wait %d again, and do not continue this work until it returns.", reviewID),
	}
	// The review's own fields come from show, which reads without side
	// effects. Without them a template naming one would print nothing useful.
	status, shownRaw, err := a.client.call(ctx, http.MethodGet, fmt.Sprintf("/agent/reviews/%d", reviewID), nil)
	var shown api.ShowResult
	if err == nil && status == http.StatusOK && json.Unmarshal(shownRaw, &shown) == nil {
		result.Review = shown.Review
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
				_, _ = fmt.Fprintf(w, "surface: %s\n", surfaceText(r.Daemon.Surface))
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

// surfaceText is the surface's state as status prints it.
func surfaceText(s api.Surface) string {
	switch s.State {
	case "serving":
		return "serving at " + s.URL
	case "failed":
		return "not serving: " + s.Reason
	case "starting":
		return "starting"
	default:
		return "off; the owner sets it up with eyeball tailnet login"
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
			// A daemon that holds the lock and does not answer comes back from
			// connect as a failure naming its pid, not as nothing to stop.
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
			// The daemon closes its socket before it releases its lock, so
			// stop waits for the lock. It waits for this daemon's lock: another
			// agent's command may start a new daemon the moment the socket
			// goes, and that one is not the one asked to stop.
			var stopped api.StopResult
			if err := json.Unmarshal(raw, &stopped); err != nil || stopped.Daemon == nil {
				return errors.New("the daemon's answer to stop did not say which daemon it was")
			}
			wait := daemon.StoppingGrace + 5*time.Second
			deadline := time.Now().Add(wait)
			for {
				holder, err := daemon.ReadHolder(a.client.paths.Lock)
				if !daemon.Held(a.client.paths.Lock) || err != nil || holder.PID != stopped.Daemon.PID {
					break
				}
				if time.Now().After(deadline) {
					return errors.Newf("daemon pid %d was asked to stop and has not exited after %s", stopped.Daemon.PID, wait)
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
		Flags: []ucli.Flag{
			&ucli.StringFlag{Name: "surface-addr", Usage: "also serve the surface on this loopback address, for a person debugging, such as 127.0.0.1:8111"},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			paths, err := a.paths()
			if err != nil {
				return err
			}
			return daemon.Run(ctx, daemon.Config{
				Paths:       paths,
				Build:       a.env.Build,
				Home:        a.env.Home,
				Stderr:      a.env.Stderr,
				SurfaceAddr: cmd.String("surface-addr"),
				NewNode:     a.env.NewNode,
			})
		},
	}
}

// tailnetCommand groups the owner's commands for the tailnet.
func (a *app) tailnetCommand() *ucli.Command {
	return &ucli.Command{
		Name:     "tailnet",
		Usage:    "the owner's commands for the tailnet the surface is served on",
		Commands: []*ucli.Command{a.loginCommand()},
	}
}

// loginCommand logs this machine's node in to the tailnet. It is the owner's,
// and refuses anything that is not a person at a terminal.
func (a *app) loginCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "login",
		Usage: "log this machine in to the tailnet, so the daemon serves the surface there (the owner's, at a terminal)",
		Action: func(ctx context.Context, _ *ucli.Command) error {
			// First, before anything is read or taken. An agent has no
			// terminal, and an auth key in its environment would let the
			// login finish with nobody there.
			if !a.env.Terminal {
				return &failure{code: api.CodeOwnerOnly, message: "eyeball tailnet login needs a person at a terminal", fix: "this is the owner's step, done in a terminal on this machine; nothing in it is for an agent"}
			}
			paths, err := a.paths()
			if err != nil {
				return err
			}
			held, err := daemon.TakeLoginLock(paths)
			if errors.Is(err, daemon.ErrRunning) {
				holder, herr := daemon.ReadHolder(paths.Lock)
				if herr == nil && holder.Login {
					return &failure{code: api.CodeDaemonRunning, message: fmt.Sprintf("another tailnet login (pid %d) is running", holder.PID), fix: "finish that login, or end it, and run this again"}
				}
				return &failure{code: api.CodeDaemonRunning, message: err.Error(), fix: "run eyeball stop && eyeball tailnet login as one line, since a waiting agent's next command starts the daemon again"}
			}
			if err != nil {
				return err
			}
			defer func() { _ = held.Release() }()

			// tsnet reads an auth key and its other settings from the process
			// environment when the node starts. The terminal check keeps an
			// agent from starting a login by accident, and a pseudo-terminal
			// passes it, so an auth key inherited from an agent would still
			// enroll the node with nobody there. The login is interactive or
			// nothing.
			clearTailscaleEnv()
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			newNode := a.env.NewNode
			if newNode == nil {
				newNode = tailnet.New
			}
			note := func(format string, args ...any) {
				_, _ = fmt.Fprintf(a.env.Stderr, format+"\n", args...)
			}
			note("starting the tailnet node; if it needs a login, it prints a URL to open")
			enrolled, err := tailnet.Login(ctx, paths.Dir, newNode, note)
			if err != nil {
				return &failure{code: api.CodeLoginFailed, message: err.Error(), fix: loginFix(err)}
			}
			raw, err := json.MarshalIndent(api.LoginResult{Dir: enrolled.Dir, URL: enrolled.URL, First: enrolled.First}, "", "  ")
			if err != nil {
				return errors.Wrap(err, "encode the answer")
			}
			return a.out.emit(raw, func(w io.Writer) {
				verb := "logged in again"
				if enrolled.First {
					verb = "enrolled"
				}
				_, _ = fmt.Fprintf(w, "the tailnet node is %s, in %s\n", verb, enrolled.Dir)
				_, _ = fmt.Fprintf(w, "the next daemon serves the surface at %s\n", enrolled.URL)
			})
		},
	}
}

// clearTailscaleEnv removes every TS_ and TSNET_ variable from this process's
// environment.
func clearTailscaleEnv() {
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "TS_") || strings.HasPrefix(name, "TSNET_") {
			_ = os.Unsetenv(name)
		}
	}
}

// loginFix is what the owner does about a login that did not finish.
func loginFix(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "run eyeball tailnet login again to finish it"
	case errors.Is(err, tailnet.ErrNoMagicDNS):
		return "turn MagicDNS on in the tailnet's admin console, then run eyeball tailnet login again"
	case errors.Is(err, tailnet.ErrNoHTTPS):
		return "turn HTTPS certificates on in the tailnet's admin console, then run eyeball tailnet login again"
	default:
		return "read the messages above, then run eyeball tailnet login again"
	}
}

// docCommand prints the manual, with no daemon.
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

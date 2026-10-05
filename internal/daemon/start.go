package daemon

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/cockroachdb/errors"
)

// Start launches exe with args as a daemon detached from the caller, and
// returns the started process so the caller can notice it exiting early.
//
// Detached means its own session, stdin and stdout on /dev/null, stderr
// appended to the log, and the state directory as its working directory. A
// child that inherited the caller's stdout would hold the write end of the
// pipe an agent's harness is reading, and the harness waits for end-of-file on
// it, so the agent's first command would not return until the daemon exited.
// The working directory is set because the daemon would otherwise hold open
// whatever directory its first caller ran in, which may be a worktree that is
// deleted an hour later.
func Start(p Paths, exe string, args ...string) (*exec.Cmd, error) {
	if err := Prepare(p); err != nil {
		return nil, err
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, errors.Wrap(err, "open the null device")
	}
	defer func() { _ = devnull.Close() }()
	logFile, err := os.OpenFile(p.Log, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", p.Log)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(exe, args...)
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = logFile
	cmd.Dir = p.Dir
	cmd.Env = daemonEnv(os.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "start %s", exe)
	}
	return cmd, nil
}

// daemonEnv is the environment a started daemon gets: the starting command's,
// without any TS_ or TSNET_ variable.
//
// The daemon serves every agent and inherits the environment of whichever one
// started it. tsnet reads an auth key, a control server and a forced login from
// those variables, some at package initialization before any of the daemon's
// own code runs, so they are removed before the process exists rather than
// inside it.
func daemonEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "TS_") || strings.HasPrefix(kv, "TSNET_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

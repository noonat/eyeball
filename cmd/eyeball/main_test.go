package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// Test_endToEnd builds the binary and drives it the way an agent's harness
// does: through a pipe it reads to end-of-file, with the daemon started by the
// first command rather than by the test.
func Test_endToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	g := NewWithT(t)
	bin := filepath.Join(t.TempDir(), "eyeball")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	g.Expect(err).NotTo(HaveOccurred(), string(out))

	// The state directory is short, because a socket path may hold 107 bytes
	// and t.TempDir nests the test's name.
	state, err := os.MkdirTemp("", "ebe-")
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = os.RemoveAll(state) })
	repo := gitRepo(g, t)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"XDG_STATE_HOME=" + state,
		"EYEBALL_AGENT=e2e",
	}
	run := func(args ...string) (string, int, time.Duration) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = repo
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdout, err := cmd.StdoutPipe()
		g.Expect(err).NotTo(HaveOccurred())
		began := time.Now()
		g.Expect(cmd.Start()).To(Succeed())
		got, err := io.ReadAll(stdout)
		g.Expect(err).NotTo(HaveOccurred())
		elapsed := time.Since(began)
		code := 0
		if err := cmd.Wait(); err != nil {
			exit, ok := err.(*exec.ExitError)
			g.Expect(ok).To(BeTrue(), err.Error())
			code = exit.ExitCode()
		}
		return string(got) + stderr.String(), code, elapsed
	}
	t.Cleanup(func() { run("stop") })

	// The first command starts the daemon, and returns without the daemon
	// holding the pipe open.
	got, code, elapsed := run("request", "a.txt", "-m", "Add a second line")
	g.Expect(code).To(BeZero(), got)
	g.Expect(got).To(ContainSubstring("review 1 opened"))
	g.Expect(elapsed).To(BeNumerically("<", 5*time.Second), "the started daemon held the pipe")

	got, code, _ = run("status")
	g.Expect(code).To(BeZero(), got)
	g.Expect(got).To(ContainSubstring("review 1, round 1, waiting"))

	// A wait in the background, woken by an abandon from another command.
	waiter := exec.Command(bin, "wait", "1")
	waiter.Dir, waiter.Env = repo, env
	var waited bytes.Buffer
	waiter.Stdout = &waited
	g.Expect(waiter.Start()).To(Succeed())
	done := make(chan error, 1)
	go func() { done <- waiter.Wait() }()
	g.Consistently(done, 500*time.Millisecond).ShouldNot(Receive(), "wait returned before anything happened")
	got, code, _ = run("abandon", "1")
	g.Expect(code).To(BeZero(), got)
	g.Eventually(done, 5*time.Second).Should(Receive(BeNil()))
	g.Expect(waited.String()).To(ContainSubstring("review 1: withdrawn"))

	got, code, _ = run("stop")
	g.Expect(code).To(BeZero(), got)
	g.Expect(got).To(ContainSubstring("stopped"))
	lock, err := os.OpenFile(filepath.Join(state, "eyeball", "eyeball.lock"), os.O_RDWR, 0)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = lock.Close() }()
	g.Expect(syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)).To(Succeed(), "stop returned with the lock still held")

	got, code, _ = run("stop")
	g.Expect(code).To(BeZero(), got)
	g.Expect(got).To(ContainSubstring("no daemon is running"))
}

// gitRepo builds a repository with one committed file and an edit to it.
func gitRepo(g *WithT, t *testing.T) string {
	g.THelper()
	dir := filepath.Join(t.TempDir(), "harbor")
	g.Expect(os.MkdirAll(dir, 0o700)).To(Succeed())
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(
			os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=t",
			"GIT_AUTHOR_EMAIL=t@example.invalid",
			"GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@example.invalid",
		)
		out, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred(), string(out))
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet", "--template=", "-b", "main")
	g.Expect(os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o600)).To(Succeed())
	git("add", "-A")
	commit := git("commit-tree", git("write-tree"), "-m", "base")
	git("update-ref", "refs/heads/main", commit)
	g.Expect(os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\n"), 0o600)).To(Succeed())
	return dir
}

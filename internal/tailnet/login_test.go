package tailnet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestLogin(t *testing.T) {
	g := NewWithT(t)
	state := t.TempDir()
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"eyeball.example.ts.net"}}, StateStarting, StateNeedsLogin, StateRunning)
	node.mkdir = true
	node.loginURL = "https://login.example.com/a/1234"
	var notes []string
	note := func(format string, args ...any) { notes = append(notes, fmt.Sprintf(format, args...)) }

	got, err := Login(t.Context(), state, node.construct, note)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(Enrollment{Dir: NodeDir(state), URL: "https://eyeball.example.ts.net", First: true}))
	// The URL tsnet says is what the person opens to log in. Without it the
	// login waits for a login nobody can make.
	g.Expect(strings.Join(notes, "\n")).To(ContainSubstring("https://login.example.com/a/1234"))
	// The node ran in node.new, and only the finished enrollment is state/node.
	g.Expect(node.dir).To(Equal(NewNodeDir(state)))
	g.Expect(Enrolled(state)).To(BeTrue())
	g.Expect(NewNodeDir(state)).NotTo(BeADirectory())
	g.Expect(node.closes.Load()).To(Equal(int32(1)), "the node was closed before the rename")
}

func TestLogin_again(t *testing.T) {
	g := NewWithT(t)
	state := t.TempDir()
	g.Expect(os.Mkdir(NodeDir(state), 0o700)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(NodeDir(state), "tailscaled.state"), []byte("the machine's key"), 0o600)).To(Succeed())
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"eyeball.example.ts.net"}}, StateNeedsLogin, StateRunning)
	node.mkdir = true

	got, err := Login(t.Context(), state, node.construct, discard)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.First).To(BeFalse())
	// The enrolled node logs in where it is: a fresh directory would be a
	// second machine with a new name.
	g.Expect(node.dir).To(Equal(NodeDir(state)))
	g.Expect(NewNodeDir(state)).NotTo(BeADirectory())
	kept, err := os.ReadFile(filepath.Join(NodeDir(state), "tailscaled.state"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(kept)).To(Equal("the machine's key"))
}

func TestLogin_approvalNotedOnce(t *testing.T) {
	g := NewWithT(t)
	state := t.TempDir()
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"x"}}, StateNeedsApproval, StateNeedsApproval, StateRunning)
	node.mkdir = true
	var notes []string
	note := func(format string, args ...any) { notes = append(notes, fmt.Sprintf(format, args...)) }

	_, err := Login(t.Context(), state, node.construct, note)
	g.Expect(err).NotTo(HaveOccurred())
	approvals := 0
	for _, n := range notes {
		if strings.Contains(n, "approval") {
			approvals++
		}
	}
	g.Expect(approvals).To(Equal(1))
}

func TestLogin_fails(t *testing.T) {
	cases := []struct {
		name     string
		enrolled bool
		status   Status
		want     error
	}{
		{
			name:   "a first login on a tailnet without MagicDNS",
			status: Status{CertDomains: []string{"x"}},
			want:   ErrNoMagicDNS,
		},
		{
			name:   "a first login on a tailnet without HTTPS",
			status: Status{MagicDNS: true},
			want:   ErrNoHTTPS,
		},
		{
			name:     "a login again on a tailnet that turned HTTPS off",
			enrolled: true,
			status:   Status{MagicDNS: true},
			want:     ErrNoHTTPS,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			state := t.TempDir()
			if c.enrolled {
				g.Expect(os.Mkdir(NodeDir(state), 0o700)).To(Succeed())
			}
			node := newFake(c.status, StateRunning)
			node.mkdir = true

			_, err := Login(t.Context(), state, node.construct, discard)
			g.Expect(errors.Is(err, c.want)).To(BeTrue(), fmt.Sprint(err))
			g.Expect(node.closes.Load()).To(Equal(int32(1)))
			// A failed first login leaves nothing a daemon reads as enrolled,
			// and a failed login again leaves the enrolled node where it was.
			g.Expect(Enrolled(state)).To(Equal(c.enrolled))
		})
	}
}

func TestLogin_interrupted(t *testing.T) {
	g := NewWithT(t)
	state := t.TempDir()
	// A login nobody completes: the node waits for one until the owner
	// presses ^C.
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"x"}}, StateNeedsLogin)
	node.mkdir = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)

	_, err := Login(ctx, state, node.construct, discard)
	g.Expect(errors.Is(err, context.Canceled)).To(BeTrue(), fmt.Sprint(err))
	g.Expect(node.closes.Load()).To(Equal(int32(1)))
	g.Expect(Enrolled(state)).To(BeFalse())
}

func TestLogin_reusesALeftoverNewNode(t *testing.T) {
	g := NewWithT(t)
	state := t.TempDir()
	// A first login that failed its checks left node.new, already a machine
	// on the tailnet. Logging in again after fixing the setting uses it,
	// rather than making a second machine.
	g.Expect(os.Mkdir(NewNodeDir(state), 0o700)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(NewNodeDir(state), "tailscaled.state"), []byte("the first try's key"), 0o600)).To(Succeed())
	node := newFake(Status{MagicDNS: true, CertDomains: []string{"x"}}, StateRunning)
	node.mkdir = true

	got, err := Login(t.Context(), state, node.construct, discard)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.First).To(BeTrue())
	g.Expect(node.dir).To(Equal(NewNodeDir(state)))
	kept, err := os.ReadFile(filepath.Join(NodeDir(state), "tailscaled.state"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(kept)).To(Equal("the first try's key"))
}

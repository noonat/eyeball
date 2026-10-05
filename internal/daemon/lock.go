package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
)

// ErrRunning reports that another daemon holds the lock.
var ErrRunning = errors.New("a daemon is already running")

// Holder is the daemon a lock file names.
type Holder struct {
	// PID is the holder's process id.
	PID int
	// Started is when the holder started.
	Started time.Time
	// Stopping says the holder has begun to shut down. Its socket is gone and
	// it may still be finishing a capture and closing its tailnet node, for up
	// to StoppingGrace, before it releases the lock.
	Stopping bool
}

// stoppingLine is appended to the lock file when shutdown begins.
const stoppingLine = "stopping\n"

// lock is the held single-instance lock.
type lock struct {
	f *os.File
}

// takeLock takes the exclusive lock, or returns ErrRunning naming the daemon
// that holds it.
//
// The file is opened without O_TRUNC. A loser that truncated on open would
// wipe the winner's pid before finding out it had lost. Only after the lock is
// held is the file truncated and the pid and start time written, and only
// after that does the caller unlink and bind the socket, which is what makes a
// socket left behind by a crash harmless.
func takeLock(path string, pid int, started time.Time) (*lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", path)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			// The winner may be between truncating the file and writing it,
			// in which case there is nothing to name yet.
			holder, err := ReadHolder(path)
			if err != nil {
				return nil, errors.WithStack(ErrRunning)
			}
			return nil, errors.Wrapf(ErrRunning, "pid %d, started %s", holder.PID, holder.Started.Format(time.RFC3339))
		}
		return nil, errors.Wrapf(err, "lock %s", path)
	}
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, errors.Wrapf(err, "truncate %s", path)
	}
	content := fmt.Sprintf("%d\n%s\n", pid, started.UTC().Format(time.RFC3339Nano))
	if _, err := f.WriteAt([]byte(content), 0); err != nil {
		_ = f.Close()
		return nil, errors.Wrapf(err, "write %s", path)
	}
	return &lock{f: f}, nil
}

// Held reports whether a daemon holds the lock at path.
//
// It asks by taking the lock and dropping it at once. A daemon starting in that
// instant loses the race and exits, as it would to any live daemon, and the
// command that started it starts another.
func Held(path string) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.Is(err, syscall.EWOULDBLOCK)
	}
	return false
}

// markStopping records in the lock file that shutdown has begun, so a command
// that finds no socket can tell a daemon finishing its work from one that is
// stuck.
func (l *lock) markStopping() error {
	info, err := l.f.Stat()
	if err != nil {
		return errors.Wrap(err, "inspect the lock file")
	}
	_, err = l.f.WriteAt([]byte(stoppingLine), info.Size())
	return errors.Wrap(err, "mark the lock file stopping")
}

// release gives the lock up by closing the file, which drops the flock.
func (l *lock) release() error {
	return errors.Wrap(l.f.Close(), "release the lock")
}

// ReadHolder reads the pid and start time a lock file holds.
func ReadHolder(path string) (Holder, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Holder{}, errors.Wrapf(err, "read %s", path)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	stopping := len(lines) == 3 && lines[2]+"\n" == stoppingLine
	if stopping {
		lines = lines[:2]
	}
	if len(lines) != 2 {
		return Holder{}, errors.Newf("%s holds %d lines, not 2", path, len(lines))
	}
	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return Holder{}, errors.Wrapf(err, "read the pid in %s", path)
	}
	started, err := time.Parse(time.RFC3339Nano, lines[1])
	if err != nil {
		return Holder{}, errors.Wrapf(err, "read the start time in %s", path)
	}
	return Holder{PID: pid, Started: started, Stopping: stopping}, nil
}

package daemon

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// maxSocketPath is the longest socket path Linux accepts. sun_path is 108
// bytes and holds a terminating NUL, and a longer path fails at bind with
// "invalid argument", which names nothing.
const maxSocketPath = 107

// ErrStateDirMode reports a state directory that other users can reach.
var ErrStateDirMode = errors.New("the state directory is open to other users")

// ErrSocketPath reports a socket path too long to bind.
var ErrSocketPath = errors.New("the socket path is too long")

// Paths are where the daemon keeps everything. All of it is under one
// directory, which every process computes the same way.
type Paths struct {
	// Dir is the state directory itself.
	Dir string
	// DB is the SQLite database.
	DB string
	// Blobs is the content-addressed store for captured files.
	Blobs string
	// Socket is the unix socket agents dial.
	Socket string
	// Lock is the file whose flock makes the daemon single-instance.
	Lock string
	// Log is where a started daemon's stderr goes.
	Log string
}

// Locate computes the paths from XDG_STATE_HOME, or from the home directory
// when that is unset or relative. The XDG specification says to ignore a
// relative value, and a directory that depends on the caller's working
// directory would not be one place.
func Locate(getenv func(string) string, home string) (Paths, error) {
	base := getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(base) {
		if !filepath.IsAbs(home) {
			return Paths{}, errors.Newf("cannot find the state directory: XDG_STATE_HOME is unset and the home directory %q is not absolute", home)
		}
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "eyeball")
	return Paths{
		Dir:    dir,
		DB:     filepath.Join(dir, "eyeball.db"),
		Blobs:  filepath.Join(dir, "blobs"),
		Socket: filepath.Join(dir, "eyeball.sock"),
		Lock:   filepath.Join(dir, "eyeball.lock"),
		Log:    filepath.Join(dir, "eyeball.log"),
	}, nil
}

// Check refuses a state directory that cannot be used safely, before anything
// binds or starts. The daemon runs it before binding, and the client runs it
// before starting a daemon, because a started daemon's stderr is the log file
// and a refusal there reaches nobody.
//
// A directory that does not exist yet passes and is created 0700 by Prepare.
// One that exists with group or other bits set is refused rather than
// tightened: file permissions are the whole of the agent API's access control,
// and a directory somebody opened up on purpose is not this process's to close.
func Check(p Paths) error {
	if len(p.Socket) > maxSocketPath {
		return errors.Wrapf(ErrSocketPath, "%s is %d bytes and a socket path may be at most %d; set XDG_STATE_HOME to a shorter directory", p.Socket, len(p.Socket), maxSocketPath)
	}
	info, err := os.Stat(p.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "inspect %s", p.Dir)
	}
	if !info.IsDir() {
		return errors.Newf("%s is not a directory", p.Dir)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		return errors.Wrapf(ErrStateDirMode, "%s has mode %04o; run chmod 700 %s if that is intended to be private", p.Dir, mode, p.Dir)
	}
	return nil
}

// Prepare checks the paths and creates the state directory if it is missing.
func Prepare(p Paths) error {
	if err := Check(p); err != nil {
		return err
	}
	return errors.Wrapf(os.MkdirAll(p.Dir, 0o700), "create %s", p.Dir)
}

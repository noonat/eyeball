package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// ErrInvalidDigest reports a digest that is not 64 lowercase hex characters.
// It is returned before any filesystem call, because a digest is what becomes a
// path.
var ErrInvalidDigest = errors.New("invalid digest")

// digestLen is a sha256 digest's length written as hex.
const digestLen = sha256.Size * 2

// tmpName is the directory partial writes go to. It cannot collide with a
// fan-out directory, because a fan-out name is hex and `t` is not a hex digit.
// Keeping it inside the store also keeps a rename on one filesystem, which is
// what makes the rename atomic.
const tmpName = "tmp"

// dirPerm and filePerm keep the store readable only by the user running it. It
// holds the content of whatever an agent was working on, which is nobody else's
// on a shared machine.
const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// Store holds file content addressed by the sha256 of the bytes.
type Store struct {
	root string
}

// Open prepares the store rooted at dir, creating what is not there yet.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, tmpName), dirPerm); err != nil {
		return nil, errors.Wrap(err, "create blob store")
	}
	return &Store{root: dir}, nil
}

// Put stores what r yields and returns its digest with the number of bytes
// read. Content the store already holds is left as it is.
func (s *Store) Put(r io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(filepath.Join(s.root, tmpName), "put-")
	if err != nil {
		return "", 0, errors.Wrap(err, "create blob temporary")
	}
	digest, size, err := s.commit(tmp, r)
	if err != nil {
		// Either the temporary is still there, or a later step already renamed
		// it away and this finds nothing. Both are fine: what must not happen
		// is a half-written file under a digest.
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", 0, err
	}
	return digest, size, nil
}

// Has reports whether the store already holds the content at digest.
//
// A caller that has hashed a file asks before calling Put, which is what makes
// re-capturing an unchanged file cost a read rather than a copy and two fsyncs.
// Put cannot make that saving on its own: a digest is not known until the whole
// stream has been read.
func (s *Store) Has(digest string) (bool, error) {
	path, err := s.path(digest)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, errors.Wrapf(err, "stat blob %s", digest)
	}
	return true, nil
}

// Open returns the content stored at digest, positioned at the start.
func (s *Store) Open(digest string) (io.ReadSeekCloser, error) {
	path, err := s.path(digest)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "open blob %s", digest)
	}
	return f, nil
}

// Stat returns the size in bytes of the content stored at digest.
func (s *Store) Stat(digest string) (int64, error) {
	path, err := s.path(digest)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, errors.Wrapf(err, "stat blob %s", digest)
	}
	return info.Size(), nil
}

// commit drains r through tmp and makes the result durable at its digest.
//
// The file is fsynced before the rename and the directories after it, so the
// content is on disk by the time Put hands the digest back. A caller that then
// records the digest cannot end up with a row pointing at a file that is not
// there. The cost is two fsyncs per stored file, on a workload of a few writes
// a minute.
func (s *Store) commit(tmp *os.File, r io.Reader) (string, int64, error) {
	sum := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, sum), r)
	if err != nil {
		return "", 0, errors.Wrap(err, "write blob temporary")
	}
	digest := hex.EncodeToString(sum.Sum(nil))
	path, err := s.path(digest)
	if err != nil {
		return "", 0, err
	}

	held, err := s.Has(digest)
	if err != nil {
		return "", 0, err
	}
	if held {
		// The bytes are identical and already durable, so the rename and both
		// fsyncs buy nothing.
		if err := tmp.Close(); err != nil {
			return "", 0, errors.Wrap(err, "close blob temporary")
		}
		if err := os.Remove(tmp.Name()); err != nil {
			return "", 0, errors.Wrap(err, "remove blob temporary")
		}
		return digest, size, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", 0, errors.Wrap(err, "create blob directory")
	}
	if err := tmp.Chmod(filePerm); err != nil {
		return "", 0, errors.Wrap(err, "chmod blob temporary")
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, errors.Wrap(err, "sync blob temporary")
	}
	if err := tmp.Close(); err != nil {
		return "", 0, errors.Wrap(err, "close blob temporary")
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", 0, errors.Wrap(err, "rename blob into place")
	}
	// Every level up to the root. MkdirAll can have created both fan-out
	// directories, and a directory entry is durable only once the directory
	// holding it is synced, so stopping short of the root can lose the entry
	// for <aa> while the row naming the digest is already durable. Three
	// fsyncs, on a store that writes a handful of files a minute.
	levels := []string{dir, filepath.Dir(dir), s.root}
	for _, level := range levels {
		if err := syncDir(level); err != nil {
			return "", 0, err
		}
	}
	return digest, size, nil
}

// path is where digest's content lives, and is the only place in this package
// that turns a digest into a filesystem path.
//
// A digest arrives from a database row, and once the surface exists it arrives
// from a URL. A path join over an unchecked string reads whatever the caller
// asked for, so anything that is not exactly 64 lowercase hex characters is
// refused here. Lowercase is part of it rather than tidiness: two spellings of
// one digest are two paths on a case-sensitive filesystem and one path on a
// case-insensitive one, and neither is what the store means.
func (s *Store) path(digest string) (string, error) {
	if len(digest) != digestLen {
		return "", errors.Wrapf(ErrInvalidDigest, "%q has %d characters, not %d", digest, len(digest), digestLen)
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", errors.Wrapf(ErrInvalidDigest, "%q is not lowercase hex", digest)
		}
	}
	return filepath.Join(s.root, digest[0:2], digest[2:4], digest), nil
}

// syncDir fsyncs a directory, which is what makes a rename into it survive
// power loss rather than only a process crash.
func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "open blob directory")
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return errors.Wrap(err, "sync blob directory")
	}
	return errors.Wrap(d.Close(), "close blob directory")
}

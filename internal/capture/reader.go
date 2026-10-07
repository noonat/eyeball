package capture

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/noonat/eyeball/internal/blob"
	"github.com/noonat/eyeball/internal/store"
)

// ErrNotFound reports a path that does not exist as of the round asked about.
var ErrNotFound = errors.New("no such file at that round")

// Source says where a file's bytes came from, which is what the surface labels
// them with.
type Source string

const (
	// SourceRound is the round's own capture. Frozen, exact, and the only
	// source a file under review is ever read from.
	SourceRound Source = "round"
	// SourceBase is the base commit, which is exact as well: a file the
	// capture does not hold matches the base byte for byte.
	SourceBase Source = "base"
	// SourceWorking is the working copy as it is now. Not frozen, and possibly
	// moved since the round was taken.
	SourceWorking Source = "working"
)

// batchBytes bounds how much path text one ls-tree call carries. The limit is
// the operating system's argument list, and a review can name a lot of paths.
const batchBytes = 96 << 10

// Reader reads a file as it was at one round.
type Reader struct {
	root  string
	base  string
	blobs *blob.Store
	files map[string]store.File
	// live is whether Open may fall back to the working copy, and under
	// which rules. NewReader guesses it from the root, and State.Reader sets
	// it from what Inspect found.
	live Live
	// git reports a .git entry at the root, which makes the project a git
	// project. A git project with no base lists the capture alone, where a
	// marker project lists its disk.
	git bool
	// pause runs between the steps of a live read, so a test can change the
	// disk where a racing process would. It is nil outside tests.
	pause func(step string)
	// oids caches what the base holds, path to object id, with an empty id for
	// a path the base does not have. A round is frozen, so an answer stays
	// true for as long as the reader lives.
	oids map[string]string
	// batch is how much path text one ls-tree call carries, so a test can
	// reach the chunking without naming ten thousand files.
	batch int
	// lookups counts the ls-tree calls made. Asking about a round's paths one
	// at a time is a git process per file, and nothing in the output says
	// which way it happened, so the count is what a test can hold.
	lookups int
}

// capture is the file list the reader was built from.
func (r *Reader) capture() []store.File {
	out := make([]store.File, 0, len(r.files))
	for _, f := range r.files {
		out = append(out, f)
	}
	return out
}

// NewReader reads files as of the round whose capture is files.
//
// An empty base means the round has no commit to fall back to, which is a
// project with no git and a repository with no commits yet.
//
// The reader it returns allows a live read. A root holding a .git entry reads
// live only what git does not ignore, and any other root reads its whole
// working copy. That guess knows nothing of a lost repository or of a marker
// project inside a checkout, so a reader for a page is built by State.Reader
// instead, which applies what Inspect found.
func NewReader(root, base string, blobs *blob.Store, files []store.File) *Reader {
	held := make(map[string]store.File, len(files))
	for _, f := range files {
		held[f.Path] = f
	}
	live := LiveOpen
	git, err := holds(root, gitDir)
	if err != nil || git {
		// A root that cannot be looked at asks git as well, and a failure
		// there refuses the read rather than serving the root unfiltered.
		live, git = LiveIgnoreRules, true
	}
	return &Reader{
		root:  root,
		base:  base,
		blobs: blobs,
		files: held,
		live:  live,
		git:   git,
		oids:  map[string]string{},
		batch: batchBytes,
	}
}

// Frozen reads a path as of the round, from the capture or from the base.
//
// It is what the diff uses. A path in neither does not exist as of that round,
// and reading the working copy instead would put content the agent never had
// into a frozen comparison.
func (r *Reader) Frozen(ctx context.Context, p string) (io.ReadCloser, Source, error) {
	clean, err := CleanPath(p)
	if err != nil {
		return nil, "", err
	}
	return r.frozen(ctx, clean)
}

// frozen is Frozen with the path already checked.
func (r *Reader) frozen(ctx context.Context, clean string) (io.ReadCloser, Source, error) {
	if f, held := r.files[clean]; held {
		if f.Digest == "" {
			// The round recorded the path as deleted, so it does not exist as
			// of that round. Falling through to the base would resurrect it.
			return nil, "", errors.Wrapf(ErrNotFound, "%s was deleted", clean)
		}
		rc, err := r.blobs.Open(f.Digest)
		if err != nil {
			return nil, "", errors.Wrapf(err, "read %s from the round", clean)
		}
		return rc, SourceRound, nil
	}
	return r.fromBase(ctx, clean)
}

// Size is the length in bytes of a path as of the round, from the capture or
// from the base, without reading it. It is what says a file is too large to
// send before anything is sent.
func (r *Reader) Size(ctx context.Context, p string) (int64, error) {
	clean, err := CleanPath(p)
	if err != nil {
		return 0, err
	}
	if f, held := r.files[clean]; held {
		if f.Digest == "" {
			return 0, errors.Wrapf(ErrNotFound, "%s was deleted", clean)
		}
		return f.Size, nil
	}
	if r.base == "" {
		return 0, errors.Wrapf(ErrNotFound, "%s, and the round has no base", clean)
	}
	oid, err := r.baseOID(ctx, clean)
	if err != nil {
		return 0, err
	}
	if oid == "" {
		return 0, errors.Wrapf(ErrNotFound, "%s is not in the base", clean)
	}
	out, err := run(ctx, r.root, "cat-file", "-s", oid)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "size of object %s", oid)
	}
	return n, nil
}

// Deleted reports whether the round records p as deleted.
//
// A deleted path is a file the page shows by its removed lines, whatever the
// working copy holds at that path now. The round is what is being read, and a
// file put back on disk is a later fact.
func (r *Reader) Deleted(p string) bool {
	clean, err := CleanPath(p)
	if err != nil {
		return false
	}
	f, held := r.files[clean]
	return held && f.Digest == ""
}

// Open reads a path as of the round, falling back to the working copy.
//
// It is what browsing uses. The third source is not frozen and says so through
// the Source it returns, because the alternative is showing nothing, and a
// reviewer who cannot look up a definition goes to a computer. A reader built
// by State.Reader skips that source wherever Inspect refused live reads, and a
// path the round does not hold is then not found.
func (r *Reader) Open(ctx context.Context, p string) (io.ReadCloser, Source, error) {
	clean, err := CleanPath(p)
	if err != nil {
		return nil, "", err
	}
	rc, source, err := r.frozen(ctx, clean)
	if err == nil {
		return rc, source, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, "", err
	}
	if _, held := r.files[clean]; held {
		// Recorded as deleted by the round. The working copy may have it back,
		// and showing that would be showing a file the round says is gone.
		return nil, "", err
	}
	return r.fromWorking(ctx, clean)
}

// fromBase reads a path out of the base commit.
func (r *Reader) fromBase(ctx context.Context, clean string) (io.ReadCloser, Source, error) {
	if r.base == "" {
		return nil, "", errors.Wrapf(ErrNotFound, "%s, and the round has no base", clean)
	}
	oid, err := r.baseOID(ctx, clean)
	if err != nil {
		return nil, "", err
	}
	if oid == "" {
		return nil, "", errors.Wrapf(ErrNotFound, "%s is not in the base", clean)
	}
	rc, err := r.catFile(ctx, oid)
	if err != nil {
		return nil, "", err
	}
	return rc, SourceBase, nil
}

// baseOID is the object the base holds at a path, empty where it holds none.
func (r *Reader) baseOID(ctx context.Context, clean string) (string, error) {
	if oid, known := r.oids[clean]; known {
		return oid, nil
	}
	if err := r.loadOIDs(ctx, []string{clean}); err != nil {
		return "", err
	}
	return r.oids[clean], nil
}

// loadOIDs asks the base what it holds at each path and remembers the answer.
//
// The paths go on the command line, so the calls are chunked to keep the
// argument list inside what the operating system takes.
//
// --literal-pathspecs, a top-level flag rather than an ls-tree one, is what
// stops a leading colon from being read as pathspec magic. Confirmed against
// git 2.43: without it a file named :weird.txt matches nothing and reads as
// absent from the base, which is the silent kind of wrong. Wildcards are not
// the risk here, and an earlier note claiming a[1].txt was read as a character
// class was wrong: both spellings find the file either way.
func (r *Reader) loadOIDs(ctx context.Context, paths []string) error {
	for _, chunk := range chunks(paths, r.batch) {
		r.lookups++
		args := []string{"--literal-pathspecs", "ls-tree", "-r", "-z", "--full-tree", r.base, "--"}
		out, err := run(ctx, r.root, append(args, chunk...)...)
		if err != nil {
			return err
		}
		if err := r.readTree(string(out)); err != nil {
			return err
		}
		for _, p := range chunk {
			if _, known := r.oids[p]; !known {
				// ls-tree omits what the base does not have, and an empty id
				// is how that is remembered rather than asked again.
				r.oids[p] = ""
			}
		}
	}
	return nil
}

// readTree takes the object ids out of an ls-tree listing.
//
// An entry that is not a blob is treated as absent. A gitlink answers with type
// commit, and cat-file blob on a commit id fails, so a submodule recorded by a
// round displays as missing rather than as an error.
func (r *Reader) readTree(out string) error {
	for _, entry := range splitNUL(out) {
		meta, p, found := strings.Cut(entry, "\t")
		if !found {
			return errors.Newf("ls-tree entry %q has no path", entry)
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			return errors.Newf("ls-tree entry %q has %d fields, not 3", meta, len(fields))
		}
		if fields[1] != "blob" {
			r.oids[clean(p)] = ""
			continue
		}
		r.oids[clean(p)] = fields[2]
	}
	return nil
}

// catFile streams one object out of the repository.
func (r *Reader) catFile(ctx context.Context, oid string) (io.ReadCloser, error) {
	cmd := gitCommand(ctx, r.root, []string{"cat-file", "blob", oid})
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrapf(err, "read object %s", oid)
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "read object %s", oid)
	}
	return &objectReader{stdout: stdout, cmd: cmd, stderr: &stderr, oid: oid}, nil
}

// objectReader streams one git object and reaps the process behind it.
type objectReader struct {
	stdout io.ReadCloser
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	oid    string
	whole  bool
}

// Read passes the object's bytes through, noting when they run out.
func (o *objectReader) Read(p []byte) (int, error) {
	n, err := o.stdout.Read(p)
	if errors.Is(err, io.EOF) {
		o.whole = true
	}
	return n, err
}

// Close reaps the process, and reports a failure only where the whole object
// was read.
//
// A caller that stops early is ordinary: a browse of a large file renders what
// it needs and closes. Killing git there is the point, and its complaint about
// the broken pipe says nothing. A stream that ended on its own and then exited
// non-zero is different, and silently truncating a file a reviewer is reading
// is the failure worth catching.
func (o *objectReader) Close() error {
	_ = o.stdout.Close()
	if !o.whole {
		_ = o.cmd.Process.Kill()
		_ = o.cmd.Wait()
		return nil
	}
	if err := o.cmd.Wait(); err != nil {
		return errors.Wrapf(err, "read object %s: %s", o.oid, message(o.stderr))
	}
	return nil
}

// CleanPath is p as a slash-separated path inside the project, or
// ErrNotFound where p names nothing a page may read.
//
// The path arrives from a round's file list and from an address, and a join
// over an unchecked string reads whatever the caller asked for. It is exported
// so a handler cleans a path the same way the reads in this package do, and
// the two agree on what a path names.
func CleanPath(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		// %00 in an address arrives as a NUL. The kernel and exec both refuse
		// one with "invalid argument", which is no answer a page can show.
		return "", errors.Wrapf(ErrNotFound, "%q holds a NUL byte", p)
	}
	cleaned := path.Clean(strings.TrimLeft(p, "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		// Cleaning would absorb the climb and answer with a different file
		// inside the root. Refusing says what happened instead, which is what
		// a caller with a bug needs and what a caller probing does not get.
		return "", errors.Wrapf(ErrNotFound, "%q is not a path inside the project", p)
	}
	segments := strings.Split(cleaned, "/")
	for _, segment := range segments {
		if segment == gitDir {
			// A capture excludes a repository's own directory, and browsing
			// has to as well, at any depth. An untracked checkout inside the
			// project keeps its own .git/config, with a remote URL and
			// whatever credentials are in it.
			return "", errors.Wrapf(ErrNotFound, "%q is inside a repository's own directory", p)
		}
	}
	return cleaned, nil
}

// chunks splits paths into groups whose text stays under a byte budget.
func chunks(paths []string, budget int) [][]string {
	var out [][]string
	var group []string
	size := 0
	for _, p := range paths {
		if len(group) > 0 && size+len(p) > budget {
			out = append(out, group)
			group, size = nil, 0
		}
		group = append(group, p)
		size += len(p) + 1
	}
	if len(group) > 0 {
		out = append(out, group)
	}
	return out
}

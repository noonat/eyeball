package server

import (
	"context"
	"fmt"
	"image"
	// Registered for image.DecodeConfig, which reads a caption's dimensions
	// from the header of each format the standard library knows.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/cockroachdb/errors"

	"github.com/noonat/eyeball/internal/capture"
	"github.com/noonat/eyeball/internal/kind"
)

// rendering is a file the browser shows itself, as the rendered template
// prints it: frames before over after, one frame holding a page or a PDF, or
// the line naming a file the surface does not show.
type rendering struct {
	// Sides are the images or players, before over after.
	Sides []renderedSide
	// Frame is the address of a page or a PDF shown in a frame, and empty
	// for none.
	Frame string
	// Sandboxed says the frame holds a page, which runs under the sandbox.
	Sandboxed bool
	// Title names the frame for a screen reader.
	Title string
	// Kind and Size caption a PDF's frame.
	Kind, Size string
	// Facts is the line naming the file, and nil where it is shown.
	Facts *facts
}

// renderedSide is one image or player.
type renderedSide struct {
	// Label is Before, After, New or Deleted, or the kind's name where
	// nothing is compared.
	Label string
	// Tint is was or now, matching the diff's palette, and empty where
	// nothing is compared.
	Tint string
	// Caption is the dimensions and size, or the kind's name and size.
	Caption string
	// Src is the side's bytes, through the raw route.
	Src string
	// Element is img, audio or video.
	Element string
}

// facts is the line naming a file the surface does not show.
type facts struct {
	// Kind and Size say what it is and how large.
	Kind, Size string
	// Hint says why it is not shown, and is empty for a file that has no
	// rendering at all.
	Hint string
	// Load is the file's page with it sent anyway, for a file past the send
	// limit, and empty otherwise.
	Load string
}

// sideReader is one side of a comparison: what reads it and the round number
// its raw address names, 0 for the base.
type sideReader struct {
	reader *capture.Reader
	number int
}

// rawHref is a path's bytes as of a round, through the raw route.
func rawHref(reviewID int64, number int, p string) string {
	return fmt.Sprintf("/reviews/%d/raw/%d/%s", reviewID, number, escapePath(p))
}

// renderShown is a file shown as itself, or SVG in its Image view: the sides
// that differ, each from its own round's raw address, or the frame for a
// PDF. A file past the send limit on either side is the line naming it. It
// is nil where neither side holds the file.
//
// changed says the file moved between the two sides. The caller decides it as
// the rest of its page does, so a card and its file page agree about a file.
func (s *Server) renderShown(ctx context.Context, at fileAt, before, after sideReader, changed bool) (*rendering, error) {
	f := at.file
	afterSize, err := sizeOf(ctx, after.reader, at.path)
	if err != nil {
		return nil, err
	}
	beforeSize := int64(-1)
	if changed {
		if beforeSize, err = sizeOf(ctx, before.reader, at.path); err != nil {
			return nil, err
		}
	}
	if (afterSize > kind.MaxSend || beforeSize > kind.MaxSend) && !at.load {
		return &rendering{Facts: &facts{Kind: f.Name, Size: bytesText(max(afterSize, beforeSize)), Hint: pastSendLimit, Load: at.loadHref()}}, nil
	}

	if f.Kind == kind.PDF {
		if afterSize < 0 {
			return nil, nil
		}
		return &rendering{Frame: at.raw(after.number), Title: at.path, Kind: f.Name, Size: bytesText(afterSize)}, nil
	}

	type planned struct {
		from        sideReader
		size        int64
		label, tint string
	}
	var plan []planned
	switch {
	case !changed && afterSize >= 0:
		plan = []planned{{from: after, size: afterSize, label: f.Name, tint: ""}}
	case !changed:
	case beforeSize >= 0 && afterSize >= 0:
		plan = []planned{{from: before, size: beforeSize, label: "Before", tint: "was"}, {from: after, size: afterSize, label: "After", tint: "now"}}
	case afterSize >= 0:
		plan = []planned{{from: after, size: afterSize, label: "New", tint: "now"}}
	case beforeSize >= 0:
		plan = []planned{{from: before, size: beforeSize, label: "Deleted", tint: "was"}}
	}
	if len(plan) == 0 {
		return nil, nil
	}
	r := &rendering{}
	for _, p := range plan {
		side, err := s.side(ctx, at, p.from, p.size, p.label, p.tint)
		if err != nil {
			return nil, err
		}
		r.Sides = append(r.Sides, side)
	}
	return r, nil
}

// framePage is a page in its Page view: the round's own side in a sandboxed
// frame, and nil where the round does not hold it. The Source view is where
// the change is marked.
func (s *Server) framePage(ctx context.Context, at fileAt, now *capture.Reader) (*rendering, error) {
	size, err := sizeOf(ctx, now, at.path)
	if err != nil || size < 0 {
		return nil, err
	}
	if size > kind.MaxSend && !at.load {
		return &rendering{Facts: &facts{Kind: at.file.Name, Size: bytesText(size), Hint: pastSendLimit, Load: at.loadHref()}}, nil
	}
	return &rendering{Frame: at.raw(at.round.Number), Sandboxed: true, Title: at.path}, nil
}

// sandboxNotice is the banner above a page in its Page view.
var sandboxNotice = notice{Text: "Runs with no access to this surface. A page an agent wrote is a program, not a document."}

// nameBinary is the line naming a file with no rendering, and its size where
// the round holds it.
func nameBinary(ctx context.Context, at fileAt, now *capture.Reader) (*rendering, error) {
	size, err := sizeOf(ctx, now, at.path)
	if err != nil {
		return nil, err
	}
	f := &facts{Kind: binaryName(at.file)}
	if size >= 0 {
		f.Size = bytesText(size)
	}
	return &rendering{Facts: f}, nil
}

// binaryName is what the line calls a binary file. Only a file whose name
// says it is binary is named by its kind. A markdown or source file holding
// binary bytes is not what its name says.
func binaryName(f kind.File) string {
	if f.Kind == kind.Other {
		return f.Name
	}
	return "Binary file"
}

// side describes one side: its address, and a caption of its dimensions and
// size for an image, or its kind and size for a recording.
func (s *Server) side(ctx context.Context, at fileAt, from sideReader, size int64, label, tint string) (renderedSide, error) {
	out := renderedSide{Label: label, Tint: tint, Src: at.raw(from.number), Element: "img"}
	switch at.file.Kind {
	case kind.Audio:
		out.Element = "audio"
	case kind.Video:
		out.Element = "video"
	}
	if out.Element != "img" {
		out.Caption = at.file.Name + " · " + bytesText(size)
		return out, nil
	}
	out.Caption = bytesText(size)
	w, h, err := dimensions(ctx, from.reader, at.path)
	if err != nil {
		return renderedSide{}, err
	}
	if w > 0 {
		out.Caption = fmt.Sprintf("%d × %d · %s", w, h, out.Caption)
	}
	return out, nil
}

// dimensions reads an image's width and height from its header, and zero for
// a format the standard library does not decode, or bytes that are not the
// image their name says.
func dimensions(ctx context.Context, reader *capture.Reader, p string) (int, int, error) {
	rc, _, err := reader.Frozen(ctx, p)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = rc.Close() }()
	cfg, _, err := image.DecodeConfig(rc)
	if err != nil {
		return 0, 0, nil
	}
	return cfg.Width, cfg.Height, nil
}

// sizeOf is a path's size as of a reader's round, and -1 where the round
// does not hold it.
func sizeOf(ctx context.Context, reader *capture.Reader, p string) (int64, error) {
	n, err := reader.Size(ctx, p)
	if errors.Is(err, capture.ErrNotFound) {
		return -1, nil
	}
	return n, err
}

// bytesText is a size as a person reads it, in powers of a thousand as a
// phone's file browser shows them.
func bytesText(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f kB", float64(n)/1000)
	case n < 1000*1000*1000:
		return fmt.Sprintf("%.1f MB", float64(n)/(1000*1000))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1000*1000*1000))
}

// modeLine says how a path's mode differs between two readers: a file whose
// mode changed, with each side's permissions, or an added file that is not a
// regular one. It is empty where the mode did not change. A file can change
// mode without a byte moving, and eyeball check compares the mode, so the page
// has to say what the round carries.
func modeLine(ctx context.Context, before, after *capture.Reader, p string) (string, error) {
	was, err := before.Mode(ctx, p)
	if err != nil {
		return "", err
	}
	is, err := after.Mode(ctx, p)
	if err != nil {
		return "", err
	}
	switch {
	case is == "" || was == is:
		return "", nil
	case was == "" && is == modeRegular:
		return "", nil
	case was == "":
		return "new " + modeWords(is), nil
	}
	return modeWords(was) + modePermissions(was) + " → " + modeWords(is) + modePermissions(is), nil
}

// The git modes the page names.
const (
	modeRegular    = "100644"
	modeExecutable = "100755"
	modeSymlink    = "120000"
)

// modeWords is a git mode as a reader names it.
func modeWords(mode string) string {
	switch mode {
	case modeRegular:
		return "regular file"
	case modeExecutable:
		return "executable file"
	case modeSymlink:
		return "symlink"
	}
	return "mode " + mode
}

// modePermissions is a file's permissions in parentheses, as the owner asked
// for over git's six digits, and nothing for a symlink, which has none.
func modePermissions(mode string) string {
	if mode == modeSymlink || len(mode) != 6 {
		return ""
	}
	return " (" + mode[3:] + ")"
}

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
	"github.com/noonat/eyeball/internal/store"
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

// renderShown fills in a file shown as itself, or SVG in its Image view: the
// sides that differ, each from its own round's raw address, or the frame for
// a PDF. A file past the send limit on either side is the line naming it.
//
// Whether the file moved is decided from the two captures' digests, as
// compareView decides it, so the page and the review agree about a file.
func (s *Server) renderShown(ctx context.Context, view *filePage, at fileAt, st capture.State, now *capture.Reader, previous, current []store.File) error {
	f := at.file
	after := sideReader{reader: now, number: at.round.Number}
	before := sideReader{reader: st.Reader(s.blobs, previous), number: at.previousNumber()}
	afterSize, err := sizeOf(ctx, now, at.path)
	if err != nil {
		return err
	}
	changed := st.Measurable() && capture.Differs(previous, current, at.path)
	beforeSize := int64(-1)
	if changed {
		if beforeSize, err = sizeOf(ctx, before.reader, at.path); err != nil {
			return err
		}
	}
	if afterSize > kind.MaxSend || beforeSize > kind.MaxSend {
		view.Rendered = &rendering{Facts: &facts{Kind: f.Name, Size: bytesText(max(afterSize, beforeSize)), Hint: tooLargeToSend}}
		return nil
	}

	if f.Kind == kind.PDF {
		if afterSize < 0 {
			return nil
		}
		view.Rendered = &rendering{Frame: rawHref(at.review.ID, after.number, at.path), Title: at.path, Kind: f.Name, Size: bytesText(afterSize)}
		return nil
	}

	r := &rendering{}
	if !changed {
		if afterSize < 0 {
			return nil
		}
		side, err := s.side(ctx, at, after, afterSize, f.Name, "")
		if err != nil {
			return err
		}
		r.Sides = append(r.Sides, side)
		view.Rendered = r
		return nil
	}
	switch {
	case beforeSize >= 0 && afterSize >= 0:
		was, err := s.side(ctx, at, before, beforeSize, "Before", "was")
		if err != nil {
			return err
		}
		is, err := s.side(ctx, at, after, afterSize, "After", "now")
		if err != nil {
			return err
		}
		r.Sides = append(r.Sides, was, is)
	case afterSize >= 0:
		is, err := s.side(ctx, at, after, afterSize, "New", "now")
		if err != nil {
			return err
		}
		r.Sides = append(r.Sides, is)
	case beforeSize >= 0:
		was, err := s.side(ctx, at, before, beforeSize, "Deleted", "was")
		if err != nil {
			return err
		}
		r.Sides = append(r.Sides, was)
	default:
		return nil
	}
	view.Rendered = r
	view.Deleted = ""
	return nil
}

// framePage fills in a page in its Page view: the round's own side in a
// sandboxed frame. The Source view is where the change is marked.
func (s *Server) framePage(ctx context.Context, view *filePage, at fileAt, now *capture.Reader) error {
	size, err := sizeOf(ctx, now, at.path)
	if err != nil || size < 0 {
		return err
	}
	if size > kind.MaxSend {
		view.Rendered = &rendering{Facts: &facts{Kind: at.file.Name, Size: bytesText(size), Hint: tooLargeToSend}}
		return nil
	}
	view.Rendered = &rendering{Frame: rawHref(at.review.ID, at.round.Number, at.path), Sandboxed: true, Title: at.path}
	view.Notices = append(view.Notices, notice{Text: "Runs with no access to this surface. A page an agent wrote is a program, not a document."})
	return nil
}

// side describes one side: its address, and a caption of its dimensions and
// size for an image, or its kind and size for a recording.
func (s *Server) side(ctx context.Context, at fileAt, from sideReader, size int64, label, tint string) (renderedSide, error) {
	out := renderedSide{Label: label, Tint: tint, Src: rawHref(at.review.ID, from.number, at.path), Element: "img"}
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

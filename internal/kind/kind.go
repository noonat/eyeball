package kind

import (
	"path"
	"strings"

	"github.com/noonat/eyeball/internal/diff"
)

// Kind is what a file is, which decides how it opens.
type Kind int

const (
	// Text is source code and every other text format without a rendering.
	Text Kind = iota
	// Markdown renders as a document.
	Markdown
	// Page is HTML and SVG: a rendering that can carry script.
	Page
	// Image is a picture a browser displays.
	Image
	// Audio is a recording a browser plays.
	Audio
	// Video is a recording a browser plays.
	Video
	// PDF is a document a browser's own viewer shows.
	PDF
	// Other is anything a browser cannot display.
	Other
)

const (
	// ViewMarked is text with the change marked.
	ViewMarked = "marked"
	// ViewPlain is text with nothing marked.
	ViewPlain = "plain"
	// ViewDocument is markdown rendered.
	ViewDocument = "document"
	// ViewSource is markdown or a page as text.
	ViewSource = "source"
	// ViewPage is HTML rendered in the sandbox.
	ViewPage = "page"
	// ViewImage is SVG rendered as an image.
	ViewImage = "image"
)

const (
	// ChoiceRendered is the agent asking for a file's rendering first.
	ChoiceRendered = "rendered"
	// ChoiceSource is the agent asking for a file's source first.
	ChoiceSource = "source"
)

// MaxSend is the largest file the surface sends to a phone.
//
// Twenty megabytes takes sixteen seconds at ten megabits a second. It is a
// guess written down so it can be corrected. It counts in thousands, as the
// sizes the surface prints do, so a file past it never prints as 20.0 MB.
const MaxSend = 20 * 1000 * 1000

// File is what a file is and how its bytes are described.
type File struct {
	// Kind decides how the file opens.
	Kind Kind
	// Type is the content type its bytes are served with.
	Type string
	// Name says what it is in words, for the line that names a file the
	// surface does not show.
	Name string
	// rendered is the view its rendering opens in, for a kind with both a
	// rendering and a source.
	rendered string
}

// text is the content type of a text file with no type of its own.
const text = "text/plain; charset=utf-8"

// byExtension is every extension that decides a file's kind. A fixed table,
// because mime.TypeByExtension reads the host's /etc/mime.types and the same
// binary would answer differently on two machines.
var byExtension = map[string]File{
	".md":       {Kind: Markdown, Type: "text/markdown; charset=utf-8", Name: "Markdown document", rendered: ViewDocument},
	".markdown": {Kind: Markdown, Type: "text/markdown; charset=utf-8", Name: "Markdown document", rendered: ViewDocument},

	".html": {Kind: Page, Type: "text/html; charset=utf-8", Name: "HTML page", rendered: ViewPage},
	".htm":  {Kind: Page, Type: "text/html; charset=utf-8", Name: "HTML page", rendered: ViewPage},
	".svg":  {Kind: Page, Type: "image/svg+xml", Name: "SVG image", rendered: ViewImage},

	".png":  {Kind: Image, Type: "image/png", Name: "PNG image"},
	".jpg":  {Kind: Image, Type: "image/jpeg", Name: "JPEG image"},
	".jpeg": {Kind: Image, Type: "image/jpeg", Name: "JPEG image"},
	".gif":  {Kind: Image, Type: "image/gif", Name: "GIF image"},
	".webp": {Kind: Image, Type: "image/webp", Name: "WebP image"},
	".avif": {Kind: Image, Type: "image/avif", Name: "AVIF image"},
	".heic": {Kind: Image, Type: "image/heic", Name: "HEIC image"},
	".bmp":  {Kind: Image, Type: "image/bmp", Name: "BMP image"},
	".ico":  {Kind: Image, Type: "image/x-icon", Name: "Icon"},

	".mp3":  {Kind: Audio, Type: "audio/mpeg", Name: "MP3 audio"},
	".m4a":  {Kind: Audio, Type: "audio/mp4", Name: "AAC audio"},
	".aac":  {Kind: Audio, Type: "audio/aac", Name: "AAC audio"},
	".wav":  {Kind: Audio, Type: "audio/wav", Name: "WAV audio"},
	".flac": {Kind: Audio, Type: "audio/flac", Name: "FLAC audio"},
	".ogg":  {Kind: Audio, Type: "audio/ogg", Name: "Ogg audio"},
	".oga":  {Kind: Audio, Type: "audio/ogg", Name: "Ogg audio"},
	".opus": {Kind: Audio, Type: "audio/ogg", Name: "Opus audio"},

	".mp4":  {Kind: Video, Type: "video/mp4", Name: "MP4 video"},
	".m4v":  {Kind: Video, Type: "video/mp4", Name: "MP4 video"},
	".mov":  {Kind: Video, Type: "video/quicktime", Name: "QuickTime video"},
	".webm": {Kind: Video, Type: "video/webm", Name: "WebM video"},
	".ogv":  {Kind: Video, Type: "video/ogg", Name: "Ogg video"},

	".pdf": {Kind: PDF, Type: "application/pdf", Name: "PDF document"},

	// A page's own stylesheets and scripts are served as what they are,
	// since the surface sends nosniff and a browser refuses a stylesheet
	// served as plain text.
	".css":  {Kind: Text, Type: "text/css; charset=utf-8", Name: "Stylesheet"},
	".js":   {Kind: Text, Type: "text/javascript; charset=utf-8", Name: "JavaScript"},
	".mjs":  {Kind: Text, Type: "text/javascript; charset=utf-8", Name: "JavaScript"},
	".json": {Kind: Text, Type: "application/json", Name: "JSON"},

	".zip":   {Kind: Other, Type: "application/zip", Name: "ZIP archive"},
	".gz":    {Kind: Other, Type: "application/gzip", Name: "Gzip archive"},
	".tar":   {Kind: Other, Type: "application/x-tar", Name: "Tar archive"},
	".woff":  {Kind: Other, Type: "font/woff", Name: "Web font"},
	".woff2": {Kind: Other, Type: "font/woff2", Name: "Web font"},
	".ttf":   {Kind: Other, Type: "font/ttf", Name: "TrueType font"},
	".otf":   {Kind: Other, Type: "font/otf", Name: "OpenType font"},
	".wasm":  {Kind: Other, Type: "application/wasm", Name: "WebAssembly module"},
}

// Of says what the file at p is. The extension decides, ignoring case, and a
// file whose extension is not in the table is text or other by diff's binary
// test over head, its first bytes, so the two packages agree on what is text.
func Of(p string, head []byte) File {
	if f, ok := byExtension[strings.ToLower(path.Ext(p))]; ok {
		return f
	}
	if diff.IsBinary(head) {
		return File{Kind: Other, Type: "application/octet-stream", Name: "Binary file"}
	}
	return File{Kind: Text, Type: text, Name: "Text file"}
}

// ByName says what the file at p is from its name alone, which is enough
// wherever the extension is in the table. A file it cannot place is text.
func ByName(p string) File {
	return Of(p, nil)
}

// Shown says the browser displays the file itself, with no text to read.
func (f File) Shown() bool {
	switch f.Kind {
	case Image, Audio, Video, PDF:
		return true
	}
	return false
}

// Default is the view the file opens in: the rendering for markdown, the
// source for a page, and the change marked for any other text. A file shown
// as itself, and other, have one view and return empty.
func (f File) Default() string {
	switch f.Kind {
	case Markdown:
		return ViewDocument
	case Page:
		return ViewSource
	case Text:
		return ViewMarked
	}
	return ""
}

// Rendered is the view the file's rendering opens in, for a file with both a
// rendering and a source, and empty for any other.
func (f File) Rendered() string {
	return f.rendered
}

// Choose turns the agent's choice into the view it names, and reports false
// for a choice this file has no view for: a rendering of code, or the source
// of an image.
func (f File) Choose(choice string) (string, bool) {
	switch choice {
	case ChoiceRendered:
		return f.rendered, f.rendered != ""
	case ChoiceSource:
		switch f.Kind {
		case Markdown, Page:
			return ViewSource, true
		case Text:
			return ViewMarked, true
		}
	}
	return "", false
}

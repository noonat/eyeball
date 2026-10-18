package kind

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestFile_views(t *testing.T) {
	cases := []struct {
		path     string
		def      string
		rendered string
		shown    bool
	}{
		{path: "a.go", def: ViewMarked},
		{path: "a.md", def: ViewDocument, rendered: ViewDocument},
		{path: "a.html", def: ViewSource, rendered: ViewPage},
		{path: "a.svg", def: ViewSource, rendered: ViewImage},
		{path: "a.png", shown: true},
		{path: "a.mp4", shown: true},
		{path: "a.pdf", shown: true},
		{path: "a.zip"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			g := NewWithT(t)
			f := ByName(c.path)
			g.Expect(f.Default()).To(Equal(c.def))
			g.Expect(f.Rendered()).To(Equal(c.rendered))
			g.Expect(f.Shown()).To(Equal(c.shown))
		})
	}
}

func TestFile_Choose(t *testing.T) {
	cases := []struct {
		path   string
		choice string
		view   string
		ok     bool
	}{
		{
			path:   "a.md",
			choice: ChoiceRendered,
			view:   ViewDocument,
			ok:     true,
		},
		{
			path:   "a.md",
			choice: ChoiceSource,
			view:   ViewSource,
			ok:     true,
		},
		{
			path:   "a.html",
			choice: ChoiceRendered,
			view:   ViewPage,
			ok:     true,
		},
		{
			path:   "a.html",
			choice: ChoiceSource,
			view:   ViewSource,
			ok:     true,
		},
		{
			path:   "a.svg",
			choice: ChoiceRendered,
			view:   ViewImage,
			ok:     true,
		},
		{
			path:   "a.go",
			choice: ChoiceSource,
			view:   ViewMarked,
			ok:     true,
		},
		{path: "a.go", choice: ChoiceRendered},
		{path: "a.png", choice: ChoiceSource},
		{path: "a.png", choice: ChoiceRendered},
		{path: "a.md", choice: "page"},
	}
	for _, c := range cases {
		t.Run(c.path+" "+c.choice, func(t *testing.T) {
			g := NewWithT(t)
			view, ok := ByName(c.path).Choose(c.choice)
			g.Expect(ok).To(Equal(c.ok))
			g.Expect(view).To(Equal(c.view))
		})
	}
}

func TestOf(t *testing.T) {
	cases := []struct {
		name string
		path string
		head string
		kind Kind
		typ  string
	}{
		{
			name: "markdown",
			path: "docs/product.md",
			kind: Markdown,
			typ:  "text/markdown; charset=utf-8",
		},
		{
			name: "long markdown extension",
			path: "notes.markdown",
			kind: Markdown,
			typ:  "text/markdown; charset=utf-8",
		},
		{
			name: "case is ignored",
			path: "README.MD",
			kind: Markdown,
			typ:  "text/markdown; charset=utf-8",
		},
		{
			name: "a directory's extension is not the file's",
			path: "docs.md/x.go",
			kind: Text,
			typ:  text,
		},
		{
			name: "html",
			path: "docs/design/queue.html",
			kind: Page,
			typ:  "text/html; charset=utf-8",
		},
		{
			name: "svg is a page",
			path: "icon.svg",
			kind: Page,
			typ:  "image/svg+xml",
		},
		{
			name: "png",
			path: "web/static/icons/icon-192.png",
			kind: Image,
			typ:  "image/png",
		},
		{
			name: "jpeg",
			path: "plate.JPG",
			kind: Image,
			typ:  "image/jpeg",
		},
		{
			name: "audio",
			path: "beep.mp3",
			kind: Audio,
			typ:  "audio/mpeg",
		},
		{
			name: "video",
			path: "capture.mov",
			kind: Video,
			typ:  "video/quicktime",
		},
		{
			name: "pdf",
			path: "paper.pdf",
			kind: PDF,
			typ:  "application/pdf",
		},
		{
			name: "stylesheet",
			path: "app.css",
			kind: Text,
			typ:  "text/css; charset=utf-8",
		},
		{
			name: "script",
			path: "main.js",
			kind: Text,
			typ:  "text/javascript; charset=utf-8",
		},
		{
			name: "font",
			path: "a.woff2",
			kind: Other,
			typ:  "font/woff2",
		},
		{
			name: "unknown text",
			path: "main.go",
			head: "package main\n",
			kind: Text,
			typ:  text,
		},
		{
			name: "no extension",
			path: "Makefile",
			head: "check:\n",
			kind: Text,
			typ:  text,
		},
		{
			name: "unknown binary",
			path: "eyeball.db",
			head: "SQLite format 3\x00",
			kind: Other,
			typ:  "application/octet-stream",
		},
		{
			name: "a table extension wins over content",
			path: "a.png",
			head: "not a png",
			kind: Image,
			typ:  "image/png",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			f := Of(c.path, []byte(c.head))
			g.Expect(f.Kind).To(Equal(c.kind))
			g.Expect(f.Type).To(Equal(c.typ))
			g.Expect(f.Name).NotTo(BeEmpty())
		})
	}
}

func TestOf_everyEntryIsComplete(t *testing.T) {
	for ext, f := range byExtension {
		t.Run(ext, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(f.Type).NotTo(BeEmpty())
			g.Expect(f.Name).NotTo(BeEmpty())
			g.Expect(f.rendered != "").To(Equal(f.Kind == Markdown || f.Kind == Page))
		})
	}
}

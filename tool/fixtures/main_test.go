package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/net/html"

	"github.com/noonat/eyeball/internal/view"
)

func TestGenerate_committedFilesAreCurrent(t *testing.T) {
	g := NewWithT(t)
	files, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(files).To(HaveLen(len(states) + 2*len(scenes)))
	for name, body := range files {
		have, err := os.ReadFile(filepath.Join("../..", outDir, name))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(have)).To(Equal(string(body)), "run: make api")
	}
}

func TestGenerate_everyRunIsTheSame(t *testing.T) {
	g := NewWithT(t)
	first, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	second, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second).To(Equal(first))
}

func TestGenerate_pagesCarryNoQueueFiles(t *testing.T) {
	g := NewWithT(t)
	files, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	for name := range files {
		if strings.HasPrefix(name, "queue-") {
			g.Expect(name).To(HaveSuffix(".json"), "the queue's markup files are frozen")
		}
	}
}

func TestGenerate_sceneViewsHoldTheirParts(t *testing.T) {
	g := NewWithT(t)
	files, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	strict := func(name string, into any) {
		dec := json.NewDecoder(bytes.NewReader(files[name+".json"]))
		dec.DisallowUnknownFields()
		g.Expect(dec.Decode(into)).To(Succeed(), name)
	}
	for _, sc := range scenes {
		switch {
		case strings.HasPrefix(sc.file, "project-"):
			var p view.Project
			strict(sc.file, &p)
			g.Expect(p.Copies).NotTo(BeEmpty(), sc.file)
		case strings.HasPrefix(sc.file, "tree-"):
			var f view.Files
			strict(sc.file, &f)
			g.Expect(f.Kind).To(Equal(view.FilesKindTree), sc.file)
			g.Expect(f.Tree).NotTo(BeNil(), sc.file)
			g.Expect(f.File).To(BeNil(), sc.file)
		default:
			var f view.Files
			strict(sc.file, &f)
			g.Expect(f.Kind).To(Equal(view.FilesKindFile), sc.file)
			g.Expect(f.File).NotTo(BeNil(), sc.file)
			g.Expect(f.Tree).To(BeNil(), sc.file)
		}
	}

	body := func(name string) view.FileBody {
		var f view.Files
		strict(name, &f)
		return f.File.Body
	}
	want := map[string]view.FileBody{
		"file-whole":         view.FileBodyRows,
		"file-document":      view.FileBodyDocument,
		"file-image-changed": view.FileBodyRendered,
		"file-pdf":           view.FileBodyRendered,
		"file-too-large":     view.FileBodyRendered,
		"file-image-deleted": view.FileBodyRendered,
	}
	for name, kind := range want {
		g.Expect(body(name)).To(Equal(kind), name)
	}

	var doc view.Files
	strict("file-document", &doc)
	g.Expect(string(doc.File.Document)).To(ContainSubstring("<h2"))
	var mode view.Files
	strict("file-mode", &mode)
	g.Expect(mode.File.Mode).To(ContainSubstring("executable file (755)"))
	var tooLarge view.Files
	strict("file-too-large", &tooLarge)
	g.Expect(tooLarge.File.Rendered.Facts.Load).To(HaveSuffix("?load=1"))
	var worktrees view.Project
	strict("project-worktrees", &worktrees)
	g.Expect(worktrees.Named).To(BeTrue())
	g.Expect(worktrees.Copies).To(HaveLen(2))
	var notGit view.Project
	strict("project-not-git", &notGit)
	g.Expect(notGit.Copies[0].Empty).NotTo(BeNil())
	g.Expect(notGit.Copies[0].Notices).To(HaveLen(1))
}

func TestGenerate_statesHoldTheirSections(t *testing.T) {
	g := NewWithT(t)
	files, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	read := func(name string) view.Queue {
		var q view.Queue
		g.Expect(json.Unmarshal(files[name], &q)).To(Succeed())
		return q
	}
	populated := read("queue-populated.json")
	g.Expect(populated.Waiting).To(HaveLen(3))
	g.Expect(populated.Working).To(HaveLen(2))
	g.Expect(populated.Settled).To(HaveLen(2))
	g.Expect(populated.WaitingCount).To(Equal(int32(3)))

	empty := read("queue-nothing-waiting.json")
	g.Expect(empty.Waiting).To(BeNil())
	g.Expect(empty.WaitingCount).To(BeZero())
	g.Expect(string(files["queue-nothing-waiting.json"])).To(ContainSubstring(`"waiting": null`))

	awkward := read("queue-awkward.json")
	g.Expect(awkward.Waiting).To(HaveLen(8))
	g.Expect(awkward.Waiting[0].Files).To(Equal("11 files"))
	g.Expect(awkward.Waiting[0].Age).To(Equal("4d"))
}

func TestGenerate_viewsRepeatTheTemplatesTopBar(t *testing.T) {
	g := NewWithT(t)
	files, err := Generate(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	for _, sc := range scenes {
		t.Run(sc.file, func(t *testing.T) {
			g := NewWithT(t)
			var got bar
			g.Expect(json.Unmarshal(files[sc.file+".json"], &got)).To(Succeed())
			root, err := html.Parse(bytes.NewReader(files[sc.file+".html"]))
			g.Expect(err).NotTo(HaveOccurred())
			var want bar
			var eye string
			var walk func(*html.Node)
			walk = func(n *html.Node) {
				if n.Type == html.ElementNode {
					switch {
					case n.Data == "a" && hasClass(n, "back"):
						for _, a := range n.Attr {
							if a.Key == "href" {
								want.Back = a.Val
							}
						}
						want.BackLabel = text(n)
					case n.Data == "h1" && want.Title == "":
						// The top bar's is the first. A document holds more.
						want.Title = text(n)
					case hasClass(n, "eye"):
						for _, a := range n.Attr {
							if a.Key == "aria-label" {
								eye = a.Val
							}
						}
					}
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
			walk(root)
			// A page with no eye has no count to repeat, and a shut eye is zero.
			switch count, _, _ := strings.Cut(eye, " "); {
			case eye == "":
			case count == "Nothing":
				zero := int32(0)
				want.WaitingCount = &zero
			default:
				n, err := strconv.Atoi(count)
				g.Expect(err).NotTo(HaveOccurred(), eye)
				waiting := int32(n)
				want.WaitingCount = &waiting
			}
			g.Expect(got).To(Equal(want))
			g.Expect(got.Title).NotTo(BeEmpty())
		})
	}
}

// bar is what a page's top bar says, which a view repeats in fields.
type bar struct {
	Title        string `json:"title"`
	Back         string `json:"back"`
	BackLabel    string `json:"back_label"`
	WaitingCount *int32 `json:"waiting_count"`
}

// text is the text under a node.
func text(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(text(c))
	}
	return b.String()
}

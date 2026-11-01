package server

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"github.com/noonat/eyeball/internal/api"
	"github.com/noonat/eyeball/internal/diff"
	"github.com/noonat/eyeball/internal/render"
	"github.com/noonat/eyeball/internal/view"
)

// viewScoped says a surface page pattern is one whose view is in scope: a
// project's pages and every file address.
func viewScoped(pattern string) bool {
	_, path, _ := strings.Cut(pattern, " ")
	return strings.HasPrefix(path, "/projects/") || strings.Contains(path, "/files/")
}

// viewFixture is a surface with two projects, each with one review, so an
// address can name the other project's working copy.
type viewFixture struct {
	*fixture
	review, other int64
	project       int64
	// projectCopy and otherCopy are a working copy of each project.
	projectCopy, otherCopy int64
}

// newViewFixture opens a review on a repository with a directory, and a second
// review on a second repository.
func newViewFixture(g *WithT, t *testing.T) viewFixture {
	g.THelper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	f := newFixture(g, t)
	repo := gitRepo(g, filepath.Join(t.TempDir(), "harbor"))
	write(g, repo, "internal/b.txt", "inside\n")
	review := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: repo}, Paths: []string{"a.txt", "internal"}, Note: "first"}).Review.ID
	elsewhere := gitRepo(g, filepath.Join(t.TempDir(), "field-notes"))
	other := f.request(g, t, api.RequestBody{Caller: api.Caller{Dir: elsewhere}, Paths: []string{"a.txt"}, Note: "other"}).Review.ID

	rv, err := f.store.Review(t.Context(), review)
	g.Expect(err).NotTo(HaveOccurred())
	ov, err := f.store.Review(t.Context(), other)
	g.Expect(err).NotTo(HaveOccurred())
	own, err := f.store.WorkingCopies(t.Context(), rv.ProjectID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(own).NotTo(BeEmpty())
	others, err := f.store.WorkingCopies(t.Context(), ov.ProjectID)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(others).NotTo(BeEmpty())
	return viewFixture{fixture: f, review: review, other: other, project: rv.ProjectID, projectCopy: own[0].ID, otherCopy: others[0].ID}
}

// viewPath is the import path of the view package.
const viewPath = "github.com/noonat/eyeball/internal/view"

// stubImporter gives the view package its real types and every other import an
// empty package. A selector on an empty package is a type error that the check
// ignores, which is enough: the expressions it looks for are typed by the view
// package alone, and parsing the server's dependencies would cost seconds.
type stubImporter struct {
	view *types.Package
}

// Import returns the view package for its path and an empty package for any
// other.
func (i stubImporter) Import(path string) (*types.Package, error) {
	if path == viewPath {
		return i.view, nil
	}
	pkg := types.NewPackage(path, filepath.Base(path))
	pkg.MarkComplete()
	return pkg, nil
}

// checkSources type-checks one package's files, ignoring the errors an empty
// import causes.
func checkSources(g *WithT, fset *token.FileSet, path string, sources map[string]string, imp types.Importer) ([]*ast.File, *types.Package, *types.Info) {
	g.THelper()
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	var files []*ast.File
	for _, name := range names {
		file, err := parser.ParseFile(fset, name, sources[name], 0)
		g.Expect(err).NotTo(HaveOccurred(), name)
		files = append(files, file)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: imp, Error: func(error) {}}
	pkg, _ := conf.Check(path, fset, files, info)
	return files, pkg, info
}

// trustedUses lists where a package's source makes a value of view.DocumentHTML,
// as "file: function (how)". The field is the one the app inserts as markup, so
// a value of its type may only come from a conversion of rendered output, in
// one function. Both ways of making one are found: a conversion, and a
// constant, which Go converts without a call and so needs no call to find. The
// check reads types, so an alias of the type, an import under another name and
// a const declaration are found as well.
func trustedUses(g *WithT, fset *token.FileSet, files []*ast.File, info *types.Info) []string {
	g.THelper()
	isDocument := func(t types.Type) bool {
		named, ok := types.Unalias(t).(*types.Named)
		if !ok {
			return false
		}
		obj := named.Obj()
		return obj.Name() == "DocumentHTML" && obj.Pkg() != nil && obj.Pkg().Path() == viewPath
	}
	found := map[string]struct{}{}
	for _, file := range files {
		name := filepath.Base(fset.File(file.Pos()).Name())
		for _, decl := range file.Decls {
			where := name + ": (file)"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				where = name + ": " + fn.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				expr, ok := n.(ast.Expr)
				if !ok {
					return true
				}
				if call, ok := expr.(*ast.CallExpr); ok {
					if fun := info.Types[call.Fun]; fun.IsType() && isDocument(fun.Type) {
						found[where+" (conversion)"] = struct{}{}
						return false
					}
				}
				if tv, ok := info.Types[expr]; ok && tv.Value != nil && isDocument(tv.Type) {
					found[where+" (constant)"] = struct{}{}
				}
				return true
			})
		}
	}
	var out []string
	for use := range found {
		out = append(out, use)
	}
	sort.Strings(out)
	return out
}

// trustedConversions lists the uses of view.DocumentHTML in the view package's
// own files and in the server's, given each as file names and text.
func trustedConversions(g *WithT, viewSources, serverSources map[string]string) []string {
	g.THelper()
	fset := token.NewFileSet()
	files, viewPkg, info := checkSources(g, fset, viewPath, viewSources, stubImporter{})
	out := trustedUses(g, fset, files, info)
	files, _, info = checkSources(g, fset, "server", serverSources, stubImporter{view: viewPkg})
	return append(out, trustedUses(g, fset, files, info)...)
}

// readSources reads the non-test Go files of a directory.
func readSources(g *WithT, dir string) map[string]string {
	g.THelper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	g.Expect(err).NotTo(HaveOccurred())
	out := map[string]string{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		text, err := os.ReadFile(name)
		g.Expect(err).NotTo(HaveOccurred())
		out[name] = string(text)
	}
	return out
}

// appPatterns reads the patterns the app draws from the client function's
// file, as net/http writes them: a parameter is {name}, and a trailing splat
// is {path...}.
func appPatterns(g *WithT, source string) []string {
	g.THelper()
	list := regexp.MustCompile(`(?s)drawnPatterns = \[(.*?)\] as const`).FindStringSubmatch(source)
	g.Expect(list).NotTo(BeNil(), "the drawnPatterns list is not where the test reads it")
	var out []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(list[1], -1) {
		pattern := m[1]
		if pattern == "/" {
			out = append(out, "/{$}")
			continue
		}
		pattern = regexp.MustCompile(`:(\w+)`).ReplaceAllString(pattern, "{$1}")
		if rest, ok := strings.CutSuffix(pattern, "/*"); ok {
			pattern = rest + "/{path...}"
		}
		out = append(out, pattern)
	}
	sort.Strings(out)
	return out
}

// shellPatterns is the path of every route the daemon answers with the shell.
func shellPatterns() []string {
	var out []string
	for _, rt := range routes {
		if rt.shell {
			_, path, _ := strings.Cut(rt.pattern, " ")
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func Test_appPatternsReadsTheClientList(t *testing.T) {
	g := NewWithT(t)
	source := `export const drawnPatterns = [
  "/",
  "/a/:one/b/*",
  "/c/:one/d/:two/e/*",
] as const;`
	g.Expect(appPatterns(g, source)).To(Equal([]string{
		"/a/{one}/b/{path...}",
		"/c/{one}/d/{two}/e/{path...}",
		"/{$}",
	}))
}

func Test_codeRowsCarryChanges(t *testing.T) {
	g := NewWithT(t)
	before := []byte("keep\nthe old line\nlast")
	after := []byte("keep\nthe new line\nlast\n")
	result := diff.Whole(before, after)
	whole := codeRows(render.Whole("p.txt", result))
	var kinds []view.CodeRowKind
	var marked []string
	for _, row := range whole {
		kinds = append(kinds, row.Kind)
		for _, tok := range row.Tokens {
			if tok.Mark {
				marked = append(marked, fmt.Sprintf("%s %q", row.Kind, tok.Text))
			}
		}
	}
	g.Expect(kinds[0]).To(Equal(view.CodeRowKindContext))
	g.Expect(kinds).To(ContainElements(view.CodeRowKindAdd, view.CodeRowKindDel))
	g.Expect(marked).To(ContainElements(`del "old"`, `add "new"`))
	for _, row := range whole {
		switch row.Kind {
		case view.CodeRowKindAdd:
			g.Expect(row.Sign).To(Equal("+"))
		case view.CodeRowKindDel:
			g.Expect(row.Sign).To(Equal("−"))
		default:
			g.Expect(row.Sign).To(BeEmpty())
		}
	}
	var noNewline int
	for _, row := range whole {
		if row.NoNewline {
			noNewline++
		}
	}
	g.Expect(noNewline).To(Equal(1), "the old side's last line has no newline")

	// The review page's hunks start with a header row.
	hunks := codeRows(render.Diff("p.txt", result))
	g.Expect(hunks[0].Kind).To(Equal(view.CodeRowKindHunk))
	g.Expect(hunks[0].Header).To(HavePrefix("@@ "))
	g.Expect(hunks[0].Tokens).To(BeEmpty())
	g.Expect(strconv.Itoa(int(hunks[1].Number))).To(Equal("1"))
}

func Test_codeRowsCarryRoles(t *testing.T) {
	g := NewWithT(t)
	source := "package p\n\n// note\nfunc f() string {\n\treturn \"s\" + strconv.Itoa(42)\n}\n"
	rows := codeRows(render.Plain("p.go", []byte(source)))
	roles := map[view.TokenRole]string{}
	for _, row := range rows {
		g.Expect(row.Kind).To(Equal(view.CodeRowKindContext))
		joined := ""
		for _, tok := range row.Tokens {
			roles[tok.Role] = tok.Text
			joined += tok.Text
		}
		if row.Number > 0 {
			g.Expect(joined).To(Equal(strings.Split(source, "\n")[row.Number-1]), "row %d", row.Number)
		}
	}
	// Each role the view names comes out of the highlighter, so a mapping with
	// a misspelled class falls to plain and fails here.
	g.Expect(roles).To(HaveKey(view.TokenRoleKeyword))
	g.Expect(roles).To(HaveKey(view.TokenRoleComment))
	g.Expect(roles).To(HaveKey(view.TokenRoleFunction))
	g.Expect(roles).To(HaveKey(view.TokenRoleString))
	g.Expect(roles).To(HaveKey(view.TokenRoleNumber))
	g.Expect(roles).To(HaveKey(view.TokenRolePlain))
}

func Test_fileBodyChoosesInOrder(t *testing.T) {
	rendered := &rendering{Facts: &facts{Kind: "Archive"}}
	cases := []struct {
		name string
		page filePage
		want view.FileBody
	}{
		{
			name: "rows",
			page: filePage{},
			want: view.FileBodyRows,
		},
		{
			name: "a rendering",
			page: filePage{Rendered: rendered},
			want: view.FileBodyRendered,
		},
		{
			name: "the deleted box",
			page: filePage{Deleted: "gone"},
			want: view.FileBodyDeleted,
		},
		{
			name: "a document",
			page: filePage{ShowDocument: true},
			want: view.FileBodyDocument,
		},
		{
			name: "the binary line",
			page: filePage{Diff: render.File{Binary: true}},
			want: view.FileBodyBinary,
		},
		{
			name: "the too large line",
			page: filePage{Diff: render.File{TooLarge: true}},
			want: view.FileBodyTooLarge,
		},
		{
			name: "a rendering over the deleted box",
			page: filePage{Rendered: rendered, Deleted: "gone"},
			want: view.FileBodyRendered,
		},
		{
			name: "the deleted box over a document",
			page: filePage{Deleted: "gone", ShowDocument: true},
			want: view.FileBodyDeleted,
		},
		{
			name: "a document over the binary line",
			page: filePage{ShowDocument: true, Diff: render.File{Binary: true}},
			want: view.FileBodyDocument,
		},
		{
			name: "the binary line over the too large line",
			page: filePage{Diff: render.File{Binary: true, TooLarge: true}},
			want: view.FileBodyBinary,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(fileBody(c.page)).To(Equal(c.want))
		})
	}
}

func Test_pageRoutesHaveViews(t *testing.T) {
	g := NewWithT(t)
	have := map[string]struct{}{}
	for _, rt := range routes {
		if rt.side == sideSurface {
			have[rt.pattern] = struct{}{}
		}
	}
	var paired []string
	for pattern := range have {
		method, path, _ := strings.Cut(pattern, " ")
		if strings.HasPrefix(path, "/api/") || !viewScoped(pattern) {
			continue
		}
		paired = append(paired, pattern)
		_, ok := have[method+" /api"+path]
		g.Expect(ok).To(BeTrue(), "no view for %s", pattern)
	}
	sort.Strings(paired)
	g.Expect(paired).To(Equal([]string{
		"GET /projects/{project}",
		"GET /projects/{project}/files/{path...}",
		"GET /reviews/{review}/files/{path...}",
		"GET /reviews/{review}/rounds/{round}/files/{path...}",
	}))
}

func Test_shellRoutesAnswerTheShell(t *testing.T) {
	g := NewWithT(t)
	f := newViewFixture(g, t)
	fill := strings.NewReplacer("{review}", itoa(f.review), "{project}", itoa(f.project), "{round}", "1", "{path...}", "a.txt", "{$}", "")
	var asked int
	for _, rt := range routes {
		if rt.side != sideSurface || !rt.shell {
			continue
		}
		_, path, _ := strings.Cut(rt.pattern, " ")
		res := serveSurface(f.s.Surface(), http.MethodGet, fill.Replace(path))
		g.Expect(res.Code).To(Equal(http.StatusOK), path)
		g.Expect(res.Body.String()).To(ContainSubstring(`<div id="root"></div>`), path)
		g.Expect(res.Body.String()).To(ContainSubstring("/static/react.js"), path)
		asked++
	}
	g.Expect(asked).To(Equal(5))
}

func Test_shellRoutesAreTheAppsRoutes(t *testing.T) {
	g := NewWithT(t)
	source, err := os.ReadFile("../../web/src/app/routes.ts")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(shellPatterns()).To(Equal(appPatterns(g, string(source))))
}

func Test_trustedConversionsFindsEveryWay(t *testing.T) {
	const stub = "package view\n\ntype DocumentHTML string\n\ntype File struct{ Document DocumentHTML }\n"
	const head = "package server\n\nimport v \"github.com/noonat/eyeball/internal/view\"\n\n"
	cases := []struct {
		name   string
		view   string
		source string
		want   []string
	}{
		{
			name:   "none",
			view:   stub,
			source: head + "func f() string { return \"\" }\n",
			want:   nil,
		},
		{
			name:   "a call",
			view:   stub,
			source: head + "func f(s string) v.DocumentHTML { return v.DocumentHTML(s) }\n",
			want:   []string{"x.go: f (conversion)"},
		},
		{
			name:   "a call in a closure and a method",
			view:   stub,
			source: head + "type T struct{}\n\nfunc (T) m(s string) { _ = func() v.DocumentHTML { return v.DocumentHTML(s) } }\n",
			want:   []string{"x.go: m (conversion)"},
		},
		{
			name:   "an alias, then a call through it",
			view:   stub,
			source: head + "type alias = v.DocumentHTML\n\nfunc f(s string) alias { return alias(s) }\n",
			want:   []string{"x.go: f (conversion)"},
		},
		{
			name:   "a package-level value",
			view:   stub,
			source: head + "var d = v.DocumentHTML(fmt.Sprint(1))\n",
			want:   []string{"x.go: (file) (conversion)"},
		},
		{
			name:   "a constant in a composite literal",
			view:   stub,
			source: head + "func f() v.File { return v.File{Document: \"<b>x</b>\"} }\n",
			want:   []string{"x.go: f (constant)"},
		},
		{
			name:   "a constant assigned to the field",
			view:   stub,
			source: head + "func f(file *v.File) { file.Document = \"<b>x</b>\" }\n",
			want:   []string{"x.go: f (constant)"},
		},
		{
			name:   "a const declaration",
			view:   stub,
			source: head + "const x v.DocumentHTML = \"<b>x</b>\"\n",
			want:   []string{"x.go: (file) (constant)"},
		},
		{
			name:   "inside the view package",
			view:   stub + "\nfunc New(s string) DocumentHTML { return DocumentHTML(s) }\n",
			source: head,
			want:   []string{"v.go: New (conversion)"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got := trustedConversions(g, map[string]string{"v.go": c.view}, map[string]string{"x.go": c.source})
			g.Expect(got).To(Equal(c.want))
		})
	}
}

func Test_trustedFieldHasOneConversion(t *testing.T) {
	g := NewWithT(t)
	server := readSources(g, ".")
	g.Expect(server).To(HaveKey("views.go"))
	viewSources := readSources(g, "../view")
	g.Expect(viewSources).To(HaveKey(filepath.Join("..", "view", "files.go")))
	g.Expect(trustedConversions(g, viewSources, server)).To(Equal([]string{"views.go: documentHTML (conversion)"}))
}

func TestServer_Surface_filesViewKind(t *testing.T) {
	g := NewWithT(t)
	f := newViewFixture(g, t)
	review := "/reviews/" + itoa(f.review)
	trees := map[string]string{
		"the root":                  review + "/files/",
		"a directory":               review + "/files/internal/",
		"a directory with no slash": review + "/files/internal",
	}
	files := map[string]string{
		"a file":            review + "/files/a.txt",
		"a file at a round": review + "/rounds/1/files/internal/b.txt",
		"a live file":       "/projects/" + itoa(f.project) + "/files/a.txt",
	}
	check := func(t *testing.T, address string, kind view.FilesKind) {
		g := NewWithT(t)
		res := serveSurface(f.s.Surface(), http.MethodGet, "/api"+address)
		g.Expect(res.Code).To(Equal(http.StatusOK), res.Body.String())
		var got view.Files
		g.Expect(json.Unmarshal(res.Body.Bytes(), &got)).To(Succeed())
		g.Expect(got.Kind).To(Equal(kind))
		if kind == view.FilesKindTree {
			g.Expect(got.Tree).NotTo(BeNil())
			g.Expect(got.File).To(BeNil())
		} else {
			g.Expect(got.File).NotTo(BeNil())
			g.Expect(got.Tree).To(BeNil())
		}
		g.Expect(got.WaitingCount).To(PointTo(Equal(int32(2))))
	}
	for name, address := range trees {
		t.Run(name, func(t *testing.T) { check(t, address, view.FilesKindTree) })
	}
	for name, address := range files {
		t.Run(name, func(t *testing.T) { check(t, address, view.FilesKindFile) })
	}
}

func TestServer_Surface_projectView(t *testing.T) {
	g := NewWithT(t)
	f := newViewFixture(g, t)
	address := "/projects/" + itoa(f.project)
	res := serveSurface(f.s.Surface(), http.MethodGet, "/api"+address)
	g.Expect(res.Code).To(Equal(http.StatusOK), res.Body.String())
	var got view.Project
	g.Expect(json.Unmarshal(res.Body.Bytes(), &got)).To(Succeed())
	g.Expect(got.Back).To(Equal("/"))
	g.Expect(got.BackLabel).To(Equal("Queue"))
	g.Expect(got.Title).To(Equal("harbor"))
	g.Expect(got.WaitingCount).To(PointTo(Equal(int32(2))))
	g.Expect(got.Name).To(Equal("harbor"))
	g.Expect(got.Open).To(Equal(int32(1)))
	g.Expect(got.Waiting).To(HaveLen(1))
	g.Expect(got.Waiting[0].Project).To(BeEmpty())
	g.Expect(got.Copies).To(HaveLen(1))
	g.Expect(got.Copies[0].Rows).NotTo(BeEmpty())
}

func TestServer_Surface_viewsAnswer404(t *testing.T) {
	g := NewWithT(t)
	f := newViewFixture(g, t)
	review := "/reviews/" + itoa(f.review)
	project := "/projects/" + itoa(f.project)
	found := map[string]string{
		"a file":           review + "/files/a.txt",
		"the root":         review + "/files/",
		"a directory":      review + "/files/internal/",
		"an earlier round": review + "/rounds/1/files/a.txt",
		"the project":      project,
		"a listed file":    project + "/files/a.txt",
		"a working copy's": project + "/files/a.txt?copy=" + itoa(f.projectCopy),
	}
	missing := map[string]string{
		"no such review":                 "/reviews/9999/files/a.txt",
		"a review that is not a number":  "/reviews/x/files/a.txt",
		"no such round":                  review + "/rounds/9/files/a.txt",
		"a round that is not a number":   review + "/rounds/x/files/a.txt",
		"no such path":                   review + "/files/missing.txt",
		"a path git refuses":             review + "/files/.git/config",
		"no such project":                "/projects/9999",
		"a project that is not a number": "/projects/x",
		"a project's unlisted file":      project + "/files/keep.txt",
		"a file of no project":           "/projects/9999/files/a.txt",
		"no such copy":                   project + "/files/a.txt?copy=9999",
		"a copy that is not a number":    project + "/files/a.txt?copy=x",
		"another project's copy":         project + "/files/a.txt?copy=" + itoa(f.otherCopy),
	}
	check := func(t *testing.T, address string, status int) {
		g := NewWithT(t)
		// The shell cannot tell whether an address exists, so the page answers
		// it either way and the view's status decides what the app draws.
		page := serveSurface(f.s.Surface(), http.MethodGet, address)
		api := serveSurface(f.s.Surface(), http.MethodGet, "/api"+address)
		g.Expect(page.Code).To(Equal(http.StatusOK), "the page")
		g.Expect(page.Body.String()).To(ContainSubstring(`<div id="root"></div>`))
		g.Expect(api.Code).To(Equal(status), "the view: %s", api.Body.String())
		g.Expect(api.Header().Get("Content-Type")).To(Equal("application/json"))
		if status == http.StatusNotFound {
			var body view.NotFound
			g.Expect(json.Unmarshal(api.Body.Bytes(), &body)).To(Succeed())
			g.Expect(body.WaitingCount).To(PointTo(Equal(int32(2))))
		}
	}
	for name, address := range found {
		t.Run(name, func(t *testing.T) { check(t, address, http.StatusOK) })
	}
	for name, address := range missing {
		t.Run(name, func(t *testing.T) { check(t, address, http.StatusNotFound) })
	}
}

func TestServer_Surface_viewsRefusedBanner(t *testing.T) {
	g := NewWithT(t)
	f := newViewFixture(g, t)
	project, err := f.store.Project(t.Context(), f.project)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(os.RemoveAll(project.Root)).To(Succeed())

	// A path the round did not capture, in a project whose root is gone, is a
	// live read the surface refuses, and the banner says why.
	address := "/reviews/" + itoa(f.review) + "/files/not-captured.txt"
	res := serveSurface(f.s.Surface(), http.MethodGet, "/api"+address)
	g.Expect(res.Code).To(Equal(http.StatusNotFound), res.Body.String())
	var got view.NotFound
	g.Expect(json.Unmarshal(res.Body.Bytes(), &got)).To(Succeed())
	g.Expect(got.Notice).NotTo(BeNil())
	g.Expect(got.Notice.Warn).To(BeTrue())
	g.Expect(got.Notice.Heading).To(Equal("Only what this round captured can be shown."))
	g.Expect(got.Notice.Text).To(ContainSubstring("no longer exists"))
}

func TestServer_Surface_viewsWithNoCount(t *testing.T) {
	g := NewWithT(t)
	f := newViewFixture(g, t)
	f.s.waiting = func(context.Context) (int, error) {
		return 0, errors.New("the count cannot be read")
	}
	review := "/reviews/" + itoa(f.review)
	project := "/projects/" + itoa(f.project)
	found := map[string]string{
		"a file":      review + "/files/a.txt",
		"a directory": review + "/files/internal/",
		"the project": project,
		"a live file": project + "/files/a.txt",
	}
	missing := map[string]string{
		"a review that is not there":  "/reviews/9999/files/a.txt",
		"a project that is not there": "/projects/9999",
	}
	check := func(t *testing.T, address string, status int) {
		g := NewWithT(t)
		res := serveSurface(f.s.Surface(), http.MethodGet, "/api"+address)
		g.Expect(res.Code).To(Equal(status), res.Body.String())
		var body map[string]any
		g.Expect(json.Unmarshal(res.Body.Bytes(), &body)).To(Succeed())
		g.Expect(body).To(HaveKeyWithValue("waiting_count", BeNil()))
	}
	for name, address := range found {
		t.Run(name, func(t *testing.T) { check(t, address, http.StatusOK) })
	}
	for name, address := range missing {
		t.Run(name, func(t *testing.T) { check(t, address, http.StatusNotFound) })
	}
}

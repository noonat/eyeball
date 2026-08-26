package convention

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/net/html"
)

// tearoutDir is where the design tearouts live, relative to the repository root.
const tearoutDir = "docs/design"

// voidTags are the elements that never take a closing tag.
var voidTags = map[string]struct{}{
	"meta": {}, "link": {}, "br": {}, "img": {},
	"hr": {}, "input": {}, "source": {}, "col": {},
}

// cssComment matches a block comment, which is not part of any declaration.
var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// TearoutPage is one page of markup, named by the file it came from.
type TearoutPage struct {
	// Name is the path, relative to the repository root.
	Name string
	// Body is the markup, unparsed.
	Body string
}

// Tearout is docs/design read once: the pages, the stylesheets they share, and
// the icon list the stylesheet is generated from.
type Tearout struct {
	// Pages are the tearouts, in name order.
	Pages []TearoutPage
	// CSS is every stylesheet joined, with comments removed.
	CSS string
	// Icons are the names listed in icons.txt.
	Icons []string
}

// TearoutCheck is one rule over the design tearouts, named for its entry in
// docs/conventions.md.
//
// Run takes the whole directory because these rules are about agreement between
// files: a class used on a page is defined in a stylesheet somewhere, and a name
// shown on one page may not appear in prose on another.
type TearoutCheck struct {
	// Name is the rule's name, and also the directory its fixture lives in.
	Name string
	// Run reports every violation it finds.
	Run func(t Tearout) []Finding
}

// TearoutChecks is every rule this package enforces over the tearouts, in the
// order docs/conventions.md lists them.
var TearoutChecks = []TearoutCheck{
	{Name: "tearout-nesting", Run: tearoutNesting},
	{Name: "tearout-classes", Run: tearoutClasses},
	{Name: "tearout-fragments", Run: tearoutFragments},
	{Name: "tearout-icons", Run: tearoutIcons},
	{Name: "tearout-agents", Run: tearoutAgents},
	{Name: "css-declarations", Run: cssDeclarations},
	{Name: "icons-listed", Run: iconsListed},
}

// LoadTearout reads the tearout directory under the repository root.
func LoadTearout() (Tearout, error) {
	root, err := repoRoot()
	if err != nil {
		return Tearout{}, err
	}
	return ReadTearout(filepath.Join(root, tearoutDir), tearoutDir)
}

// ReadTearout reads one directory of tearouts, naming findings under a label.
//
// A missing stylesheet or icon list is empty rather than an error, so a fixture
// supplies only the files the check it belongs to actually reads.
func ReadTearout(dir string, label string) (Tearout, error) {
	out := Tearout{}
	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return out, errors.Wrapf(err, "list %s", dir)
	}
	sort.Strings(pages)
	for _, path := range pages {
		body, err := os.ReadFile(path)
		if err != nil {
			return out, errors.Wrapf(err, "read %s", path)
		}
		name := filepath.ToSlash(filepath.Join(label, filepath.Base(path)))
		out.Pages = append(out.Pages, TearoutPage{Name: name, Body: string(body)})
	}
	sheets := []string{"app.css", "tearout.css"}
	for _, sheet := range sheets {
		body, err := os.ReadFile(filepath.Join(dir, sheet))
		if err != nil {
			continue
		}
		out.CSS += cssComment.ReplaceAllString(string(body), "")
	}
	body, err := os.ReadFile(filepath.Join(dir, "icons.txt"))
	if err != nil {
		return out, nil
	}
	for _, name := range strings.Split(strings.TrimSpace(string(body)), ",") {
		if name = strings.TrimSpace(name); name != "" {
			out.Icons = append(out.Icons, name)
		}
	}
	return out, nil
}

// TearoutFixture reads one check's fixture directory under testdata.
func TearoutFixture(check string) (Tearout, error) {
	root, err := repoRoot()
	if err != nil {
		return Tearout{}, err
	}
	dir := filepath.Join(root, "internal", "convention", "testdata", check)
	if _, err := os.Stat(dir); err != nil {
		return Tearout{}, errors.Wrapf(err, "no fixture for check %q", check)
	}
	return ReadTearout(dir, filepath.ToSlash(filepath.Join("testdata", check)))
}

// markup is one token of a page, with the line it starts on.
type markup struct {
	// Kind is what the tokenizer called it.
	Kind html.TokenType
	// Tag is the element name, empty for text.
	Tag string
	// Attr are the attributes, by name.
	Attr map[string]string
	// Text is the character data, empty for a tag.
	Text string
	// Line is the 1-based line the token starts on.
	Line int
}

// tokens takes a page apart without repairing it.
//
// The tokenizer is used rather than the tree parser because a parser closes an
// unclosed tag for you, which is the fault tearoutNesting exists to report.
func tokens(body string) []markup {
	z := html.NewTokenizer(strings.NewReader(body))
	var out []markup
	line := 1
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			return out
		}
		raw := string(z.Raw())
		m := markup{Kind: kind, Line: line}
		line += strings.Count(raw, "\n")
		switch kind {
		case html.TextToken:
			m.Text = raw
		case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
			tok := z.Token()
			m.Tag = tok.Data
			m.Attr = map[string]string{}
			for _, a := range tok.Attr {
				m.Attr[a.Key] = a.Val
			}
		}
		out = append(out, m)
	}
}

// classes lists the classes a tag carries.
func (m markup) classes() []string {
	return strings.Fields(m.Attr["class"])
}

// hasClass reports whether a tag carries one class.
func (m markup) hasClass(want string) bool {
	for _, name := range m.classes() {
		if name == want {
			return true
		}
	}
	return false
}

// isVoid reports whether a tag never takes a closing tag.
func (m markup) isVoid() bool {
	_, ok := voidTags[m.Tag]
	return ok
}

// spans walks a page, reporting the depth of the innermost enclosing element
// that carries a class, or -1 when the walk is outside one.
//
// The five markup rules all need this: an icon span and an agent span both mean
// "this element and everything under it", which a flat token stream does not say
// on its own.
func spans(body string, class string, visit func(m markup, inside bool)) {
	depth := -1
	level := 0
	for _, m := range tokens(body) {
		if m.isVoid() {
			visit(m, depth >= 0)
			continue
		}
		switch m.Kind {
		case html.StartTagToken:
			if depth < 0 && m.hasClass(class) {
				depth = level
			}
			visit(m, depth >= 0)
			level++
			continue
		case html.EndTagToken:
			level--
			visit(m, depth >= 0)
			if depth >= 0 && level <= depth {
				depth = -1
			}
			continue
		}
		visit(m, depth >= 0)
	}
}

// tearoutAt names a place in a tearout, as file:line.
func tearoutAt(page TearoutPage, line int) string {
	return fmt.Sprintf("%s:%d", page.Name, line)
}

// sortedSet lists a set's members in order.
func sortedSet(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for name := range in {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

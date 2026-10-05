package render

import (
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	. "github.com/onsi/gomega"
)

func Test_fitLeavesALineItCannotReproducePlain(t *testing.T) {
	// Tokens that differ from the line by anything but the one newline a
	// lexer appends would shift every word mark after them.
	cases := []struct {
		name string
		raw  []chroma.Token
		want []token
	}{
		{
			name: "a token that changed",
			raw:  []chroma.Token{{Type: chroma.Keyword, Value: "return"}, {Type: chroma.Text, Value: " 2"}},
		},
		{
			name: "two newlines added",
			raw:  []chroma.Token{{Type: chroma.Keyword, Value: "return"}, {Type: chroma.Text, Value: " 1\n\n"}},
		},
		{
			name: "a token missing",
			raw:  []chroma.Token{{Type: chroma.Keyword, Value: "return"}},
		},
		{
			name: "tokens that reproduce the line",
			raw:  []chroma.Token{{Type: chroma.Keyword, Value: "return"}, {Type: chroma.Text, Value: " 1"}},
			want: []token{{start: 0, end: 6, role: roleKeyword}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(fit("return 1", c.raw)).To(Equal(c.want))
		})
	}
}

func Test_highlighterFindsFunctionNames(t *testing.T) {
	g := NewWithT(t)
	got := newHighlighter("a.go").tokens("func main() {")
	g.Expect(got).To(ContainElement(token{start: 5, end: 9, role: roleFunction}))
	g.Expect(got).To(ContainElement(token{start: 0, end: 4, role: roleKeyword}), "KeywordDeclaration is a keyword by its prefix")
}

func Test_highlighterGivesUpOnALexerThatDoesNotStop(t *testing.T) {
	g := NewWithT(t)
	// chroma's Jungle lexer emits empty tokens forever for a comment with no
	// newline. Unbounded, this grew past seven gigabytes in two minutes.
	done := make(chan []token, 1)
	go func() { done <- newHighlighter("monkey.jungle").tokens("# note") }()
	g.Eventually(done, 10*time.Second).Should(Receive(BeEmpty()))
}

func Test_highlighterGivesUpOnEndlessTokens(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "empty tokens", value: ""},
		{name: "tokens holding text", value: "x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			h := highlighter{lexer: endless{value: c.value}}
			done := make(chan []token, 1)
			go func() { done <- h.tokens("# note") }()
			g.Eventually(done, 10*time.Second).Should(Receive(BeEmpty()))
		})
	}
}

func Test_highlighterKeepsACarriageReturn(t *testing.T) {
	g := NewWithT(t)
	// chroma's default options turn a lone carriage return into a newline,
	// which would fail the reproduce check and leave the line plain.
	got := newHighlighter("a.go").tokens("x := 1\r")
	g.Expect(got).To(Equal([]token{{start: 5, end: 6, role: roleNumber}}))
}

func Test_highlighterLeavesLinesPlain(t *testing.T) {
	cases := []struct {
		name string
		path string
		line string
	}{
		{
			name: "a file chroma has no lexer for",
			path: "notes.unknownext",
			line: "return 42",
		},

		{
			name: "an empty line",
			path: "a.go",
			line: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(newHighlighter(c.path).tokens(c.line)).To(BeEmpty())
		})
	}
}

func Test_highlighterTokens(t *testing.T) {
	g := NewWithT(t)
	line := `	return "s" + 42 // done`
	got := newHighlighter("internal/a.go").tokens(line)
	g.Expect(got).To(Equal([]token{
		{start: 1, end: 7, role: roleKeyword},
		{start: 8, end: 11, role: roleString},
		{start: 14, end: 16, role: roleNumber},
		{start: 17, end: 24, role: roleComment},
	}))
	for _, tok := range got {
		g.Expect(tok.end).To(BeNumerically("<=", len(line)))
	}
}

func Test_highlighterTrimsTheNewlineALexerAdds(t *testing.T) {
	cases := []struct {
		name string
		path string
		line string
		want []token
	}{
		{
			name: "a TypeScript line ending in a comment",
			path: "a.ts",
			line: "const x = 1 // c",
			want: []token{{start: 0, end: 5, role: roleKeyword}, {start: 10, end: 11, role: roleNumber}, {start: 12, end: 16, role: roleComment}},
		},
		{
			name: "a Rust line ending in a comment",
			path: "a.rs",
			line: "let x = 1; // c",
			want: []token{{start: 0, end: 3, role: roleKeyword}, {start: 8, end: 9, role: roleNumber}, {start: 11, end: 15, role: roleComment}},
		},
		{
			name: "a TypeScript line ending in spaces",
			path: "a.ts",
			line: "const x = 1   ",
			want: []token{{start: 0, end: 5, role: roleKeyword}, {start: 10, end: 11, role: roleNumber}},
		},
		{
			name: "a Makefile comment",
			path: "Makefile",
			line: "# note",
			want: []token{{start: 0, end: 6, role: roleComment}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(newHighlighter(c.path).tokens(c.line)).To(Equal(c.want))
		})
	}
}

// endless is a lexer whose tokens never stop.
type endless struct {
	value string
}

// Config describes no language.
func (endless) Config() *chroma.Config {
	return &chroma.Config{Name: "endless"}
}

// Tokenise yields a comment token holding value, forever.
func (e endless) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	return func() chroma.Token { return chroma.Token{Type: chroma.Comment, Value: e.value} }, nil
}

// SetRegistry does nothing.
func (e endless) SetRegistry(*chroma.LexerRegistry) chroma.Lexer {
	return e
}

// SetAnalyser does nothing.
func (e endless) SetAnalyser(func(string) float32) chroma.Lexer {
	return e
}

// AnalyseText claims nothing.
func (endless) AnalyseText(string) float32 {
	return 0
}

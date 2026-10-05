package render

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// token is a run of a line with one role, by byte offset.
type token struct {
	start, end int
	role       string
}

// roles maps a chroma token type's name, by prefix, to the role class app.css
// defines for it. A type matching none is plain.
var roles = []struct {
	prefix string
	class  string
}{
	{prefix: "Keyword", class: roleKeyword},
	{prefix: "LiteralString", class: roleString},
	{prefix: "Comment", class: roleComment},
	{prefix: "LiteralNumber", class: roleNumber},
	{prefix: "NameFunction", class: roleFunction},
}

// highlighter highlights the lines of one file.
type highlighter struct {
	lexer chroma.Lexer
}

// newHighlighter chooses a lexer by the file's name. A name chroma does not
// know gives a highlighter that leaves every line plain.
func newHighlighter(path string) highlighter {
	return highlighter{lexer: lexers.Match(filepath.Base(path))}
}

// tokens highlights one line alone.
//
// chroma's default options turn a lone \r into \n for every lexer, which is
// why the options here leave EnsureLF off.
func (h highlighter) tokens(line string) []token {
	if h.lexer == nil || line == "" {
		return nil
	}
	it, err := h.lexer.Tokenise(&chroma.TokeniseOptions{State: "root"}, line)
	if err != nil {
		return nil
	}
	var raw []chroma.Token
	for t := it(); t != chroma.EOF; t = it() {
		// chroma's Jungle lexer, given a comment with no newline, emits empty
		// tokens forever. A lexer that has given back far more tokens than
		// the line has bytes has gone wrong, and the line is left plain.
		if len(raw) > maxTokens(line) {
			return nil
		}
		raw = append(raw, t)
	}
	return fit(line, raw)
}

// maxTokens is how many tokens a line may come back as. A token that is not
// empty holds at least one byte, and chroma's lexers emit some empty ones, so
// the bound is generous rather than tight.
func maxTokens(line string) int {
	return 4*len(line) + 16
}

// fit turns a line's chroma tokens into role tokens by byte offset.
//
// A lexer that sets EnsureNL appends a newline to what it is given, and 32 of
// chroma's do, TypeScript, C and Rust among them. Their rule for a line comment
// runs to a newline, so a line ending in a // comment comes back with the
// comment's token one byte longer than the line. That one newline is trimmed.
// Tokens that still do not join back into exactly the line leave it plain,
// since a token that grew would shift every word mark after it.
func fit(line string, raw []chroma.Token) []token {
	var joined strings.Builder
	for _, t := range raw {
		joined.WriteString(t.Value)
	}
	if n := len(raw); n > 0 && joined.String() == line+"\n" {
		raw[n-1].Value = strings.TrimSuffix(raw[n-1].Value, "\n")
	} else if joined.String() != line {
		return nil
	}
	var (
		out []token
		at  int
	)
	for _, t := range raw {
		start := at
		at += len(t.Value)
		if role := roleOf(t.Type); role != "" {
			out = append(out, token{start: start, end: at, role: role})
		}
	}
	return out
}

// roleOf is the role class for a token type, or empty for plain.
func roleOf(t chroma.TokenType) string {
	name := t.String()
	for _, r := range roles {
		if strings.HasPrefix(name, r.prefix) {
			return r.class
		}
	}
	return ""
}

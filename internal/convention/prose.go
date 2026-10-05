package convention

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// ProseCheck is one rule over a committed markdown file, named for its entry in
// docs/conventions.md.
//
// Run takes a whole file rather than a parsed tree. The prose rules are about
// wording, and markdown has no structure worth building for them beyond knowing
// which lines are prose at all.
type ProseCheck struct {
	// Name is the rule's name, and also the directory its fixture lives in.
	Name string
	// Run reports every violation it finds in one file.
	Run func(path string, body string) []Finding
}

// ProseChecks is every rule this package enforces over markdown, sorted by name.
var ProseChecks = []ProseCheck{
	{Name: "prose-dashes", Run: proseDashes},
	{Name: "prose-person", Run: prosePerson},
}

// personWords are the first and second person pronouns docs/voice.md bans.
var personWords = []string{
	"I", "I'm", "I've", "I'd", "I'll", "me", "my", "mine", "myself",
	"we", "we're", "we've", "we'd", "we'll", "us", "our", "ours", "ourselves",
	"you", "you're", "you've", "you'd", "you'll", "your", "yours", "yourself",
}

// personPattern matches any of personWords as a whole word, in any case.
var personPattern = regexp.MustCompile(`(?i)\b(?:` + strings.Join(personWords, "|") + `)\b`)

// inlineCode matches a code span, which names something rather than says it.
var inlineCode = regexp.MustCompile("`[^`\n]*`")

// quotedSpan matches a double-quoted example on one line.
var quotedSpan = regexp.MustCompile(`"[^"\n]*"`)

// prosePerson reports a first or second person pronoun in prose.
//
// This is the voice rule broken most often by accident, because the person
// writing is the one the rule is about and the pronoun arrives without being
// chosen.
func prosePerson(path string, body string) []Finding {
	var out []Finding
	p := proseOf(body)
	for _, at := range personPattern.FindAllStringIndex(p.text, -1) {
		out = append(out, Finding{
			At:    fmt.Sprintf("%s:%d", path, p.lineAt(at[0])),
			Check: "prose-person",
			What:  "first or second person: " + p.text[at[0]:at[1]],
		})
	}
	return out
}

// proseDashes reports an em dash in prose.
//
// docs/voice.md says a period beats a dash. The character is worth its own rule
// because it arrives without being typed, from a keyboard substitution or from
// text written elsewhere and pasted in.
func proseDashes(path string, body string) []Finding {
	var out []Finding
	p := proseOf(body)
	for at, r := range p.text {
		if r != '—' {
			continue
		}
		out = append(out, Finding{
			At:    fmt.Sprintf("%s:%d", path, p.lineAt(at)),
			Check: "prose-dashes",
			What:  "em dash in prose; a comma, a colon or a period says it",
		})
	}
	return out
}

// prose is a markdown file's own words, with everything else left out and each
// paragraph's wrapped lines joined back into one.
type prose struct {
	// text is the joined prose. A line break inside a paragraph becomes one
	// space. A blank line, a fenced block and a blockquote become a newline, so
	// nothing matches across the gap between two paragraphs.
	text string
	// at maps each byte of text to the line of the file it came from.
	at []int
}

// lineAt is the line an offset into text came from.
func (p prose) lineAt(offset int) int {
	if offset < 0 || offset >= len(p.at) {
		return 0
	}
	return p.at[offset]
}

// proseOf reads a markdown file down to the words this repository wrote.
//
// A fenced block is code. A blockquote is quoted material, which covers the
// annotations backlog writes into a spec when it closes an iteration. An inline
// code span names something rather than says it, and a double-quoted span is an
// example, which is how docs/voice.md states the rule against the first person
// without breaking it.
//
// The lines of a paragraph are joined before either span is stripped. Both
// patterns stop at a line break, so a quoted example that oxfmt wrapped would
// otherwise lose its exemption and the pronoun inside it would be reported.
func proseOf(body string) prose {
	var (
		text strings.Builder
		at   []int
	)
	write := func(s string, line int) {
		text.WriteString(s)
		for range len(s) {
			at = append(at, line)
		}
	}
	fenced := false
	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			write("\n", i+1)
			continue
		}
		if fenced || trimmed == "" || strings.HasPrefix(trimmed, ">") {
			write("\n", i+1)
			continue
		}
		write(trimmed+" ", i+1)
	}
	joined := text.String()
	stripped, kept := strip(joined, at)
	return prose{text: stripped, at: kept}
}

// strip removes the code and quoted spans, carrying the line map with them.
//
// A replacement would have to keep every byte to keep the map aligned, so the
// spans are cut and their line entries cut with them.
func strip(text string, at []int) (string, []int) {
	spans := append(inlineCode.FindAllStringIndex(text, -1), quotedSpan.FindAllStringIndex(text, -1)...)
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var (
		outText strings.Builder
		outAt   []int
	)
	end := 0
	for _, span := range spans {
		if span[0] < end {
			continue
		}
		outText.WriteString(text[end:span[0]])
		outAt = append(outAt, at[end:span[0]]...)
		outText.WriteString(" ")
		outAt = append(outAt, at[span[0]])
		end = span[1]
	}
	outText.WriteString(text[end:])
	outAt = append(outAt, at[end:]...)
	return outText.String(), outAt
}

// MarkdownFiles lists every committed markdown file, relative to the root.
//
// The list comes from git for the same reason the Makefile's does: a new file
// is untracked until it is added, and a check that reads only the index passes
// while checking nothing.
//
// A symlink is skipped, so a file linked under a second name is not checked
// twice. testdata is skipped because its fixtures break the rules on purpose.
func MarkdownFiles() ([]string, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "*.md")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "git ls-files")
	}
	var out []string
	for _, name := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if name == "" || strings.Contains(name, "/testdata/") {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// ReadMarkdown reads one file named relative to the repository root.
func ReadMarkdown(name string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return "", errors.Wrapf(err, "read %s", name)
	}
	return string(body), nil
}

// ProseFixture reads one check's markdown fixture under testdata.
func ProseFixture(check string) (string, string, error) {
	name := filepath.Join("internal", "convention", "testdata", check, "bad.md")
	body, err := ReadMarkdown(name)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(name), body, nil
}
